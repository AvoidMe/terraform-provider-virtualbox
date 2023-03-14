package provider

// TODO: add validation to params (at least enum params)

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	virtualboxapi "github.com/AvoidMe/terraform-provider-virtualbox/internal/virtualbox_api"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &VirtualboxVMResource{}
var _ resource.ResourceWithImportState = &VirtualboxVMResource{}

func NewVirtualboxVMResource() resource.Resource {
	return &VirtualboxVMResource{}
}

// VirtualboxVMResource defines the resource implementation.
type VirtualboxVMResource struct {
	client *http.Client
}

// VirtualboxVMResourceModel describes the resource data model.
type VirtualboxVMResourceModel struct {
	Id      types.String `tfsdk:"id"`
	Name    types.String `tfsdk:"name"`
	Image   types.String `tfsdk:"image"`
	SSHUser types.String `tfsdk:"ssh_user"`
	SSHKey  types.String `tfsdk:"ssh_key"`
	Cpu     types.Int64  `tfsdk:"cpu"`
	Memory  types.Int64  `tfsdk:"memory"`
	SSHPort types.String `tfsdk:"ssh_port"`
}

func (r *VirtualboxVMResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vm"
}

func (r *VirtualboxVMResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Virtualbox VM resource",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Example identifier",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Virtualbox vm name",
				Optional:            false,
				Required:            true,
			},
			"image": schema.StringAttribute{
				MarkdownDescription: "Path or URL to virtualbox vm image",
				Optional:            false,
				Required:            true,
			},
			"ssh_user": schema.StringAttribute{
				MarkdownDescription: "User for which ssh key will be injected. Root by default.",
				Optional:            true,
				Required:            false,
			},
			"ssh_key": schema.StringAttribute{
				MarkdownDescription: "Path to public ssh key, will be inserted into authorized_keys of guest vm",
				Optional:            true,
				Required:            false,
			},
			"cpu": schema.Int64Attribute{
				MarkdownDescription: "Virtualbox vm cpu count",
				Optional:            false,
				Required:            true,
			},
			"memory": schema.Int64Attribute{
				MarkdownDescription: "Virtualbox vm memory count (MB)",
				Optional:            false,
				Required:            true,
			},
			"ssh_port": schema.StringAttribute{
				MarkdownDescription: "Forwarded local port to guest ssh(22)",
				Computed:            true,
			},
		},
	}
}

func (r *VirtualboxVMResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*http.Client)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *http.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	r.client = client
}

func (r *VirtualboxVMResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data *VirtualboxVMResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	vmInfo, err := virtualboxapi.CreateVM(
		data.Image.ValueString(),
		data.Name.ValueString(),
		data.Memory.ValueInt64(),
		data.Cpu.ValueInt64(),
	)
	if err != nil {
		resp.Diagnostics.AddError("Error creating new vm", err.Error())
		err = virtualboxapi.DestroyVM(data.Name.ValueString())
		resp.Diagnostics.AddError("Error destroying vm", err.Error())
		return
	}

	if !data.SSHKey.IsNull() {
		vmInfo, err = virtualboxapi.ForwardLocalPort(vmInfo.ID, 22)
		if err != nil {
			resp.Diagnostics.AddError("Error forwarding local port", err.Error())
			err = virtualboxapi.DestroyVM(data.Name.ValueString())
			resp.Diagnostics.AddError("Error destroying vm", err.Error())
			return
		}
		sshUser := "root"
		if !data.SSHUser.IsNull() {
			sshUser = data.SSHUser.ValueString()
		}
		err = virtualboxapi.InjectSSHKey(vmInfo.ID, sshUser, data.SSHKey.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Error injecting ssh key", err.Error())
			err = virtualboxapi.DestroyVM(data.Name.ValueString())
			resp.Diagnostics.AddError("Error destroying vm", err.Error())
			return
		}
	}

	vmInfo, err = virtualboxapi.StartVM(
		vmInfo.ID,
		virtualboxapi.Headless, // TODO: add to schema, with default = headless
	)
	if err != nil {
		resp.Diagnostics.AddError("Error starting new vm", err.Error())
		err = virtualboxapi.DestroyVM(data.Name.ValueString())
		resp.Diagnostics.AddError("Error destroying vm", err.Error())
		return
	}

	// save into the Terraform state.
	data.Id = types.StringValue(vmInfo.ID)
	data.SSHPort = types.StringValue(vmInfo.SSHPort)

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Trace(ctx, "created a resource")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *VirtualboxVMResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data *VirtualboxVMResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	vminfo, err := virtualboxapi.GetVMInfo(data.Id.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error getting vm info", err.Error())
		return
	}
	data.SSHPort = types.StringValue(vminfo.SSHPort)

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *VirtualboxVMResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data *VirtualboxVMResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *VirtualboxVMResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data *VirtualboxVMResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	err := virtualboxapi.DestroyVM(
		data.Id.ValueString(),
	)
	if err != nil {
		tflog.Error(ctx, err.Error())
		resp.Diagnostics.AddError("Error destroying vm", err.Error())
		return
	}
}

func (r *VirtualboxVMResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
