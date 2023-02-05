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
	Id     types.String `tfsdk:"id"`
	Name   types.String `tfsdk:"name"`
	Image  types.String `tfsdk:"image"`
	Cpu    types.Int64  `tfsdk:"cpu"`
	Memory types.Int64  `tfsdk:"memory"`
	Nic    struct {
		Type          types.String `tfsdk:"type"`
		HostInterface types.String `tfsdk:"host_interface"`
	} `tfsdk:"nic"`
	IPV4Adress types.String `tfsdk:"ipv4_address"`
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
			"nic": schema.SingleNestedAttribute{
				MarkdownDescription: "Virtualbox network interface",
				Optional:            true,
				Required:            false,
				Attributes: map[string]schema.Attribute{
					"type": schema.StringAttribute{
						MarkdownDescription: "Virtualbox network type",
						Optional:            true,
						Required:            false,
					},
					"host_interface": schema.StringAttribute{
						MarkdownDescription: "Virtualbox network host interface",
						Optional:            true,
						Required:            false,
					},
				},
			},
			"ipv4_address": schema.StringAttribute{
				MarkdownDescription: "IPv4 Adress of VM",
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

	nic := virtualboxapi.VirtualboxNicInfo{
		Type:          data.Nic.Type.ValueString(),
		HostInterface: data.Nic.HostInterface.ValueString(),
	}

	vmInfo, err := virtualboxapi.CreateVM(
		data.Image.ValueString(),
		data.Name.ValueString(),
		data.Memory.ValueInt64(),
		data.Cpu.ValueInt64(),
	)
	if err != nil {
		resp.Diagnostics.AddError("Error creating new vm", err.Error())
		return
	}
	vmInfo, err = virtualboxapi.ModifyNIC(vmInfo, nic)
	if err != nil {
		resp.Diagnostics.AddError("Error setuping nic", err.Error())
		return
	}
	vmInfo, err = virtualboxapi.StartVM(
		vmInfo.ID,
		virtualboxapi.Headless, // TODO: add to schema, with default = headless
	)
	if err != nil {
		resp.Diagnostics.AddError("Error starting new vm", err.Error())
		return
	}

	// save into the Terraform state.
	data.Id = types.StringValue(vmInfo.ID)
	data.IPV4Adress = types.StringValue(vmInfo.IPv4)

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
	data.IPV4Adress = types.StringValue(vminfo.IPv4)

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

	_, err := virtualboxapi.StopVM(
		data.Id.ValueString(),
	)
	if err != nil {
		tflog.Error(ctx, err.Error())
		resp.Diagnostics.AddError("Error stopping vm", err.Error())
		return
	}
	err = virtualboxapi.DestroyVM(
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
