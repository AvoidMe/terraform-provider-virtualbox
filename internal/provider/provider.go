package provider

import (
	"context"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// Ensure VirtualboxProvider satisfies various provider interfaces.
var _ provider.Provider = &VirtualboxProvider{}

// VirtualboxProvider defines the provider implementation.
type VirtualboxProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

// VirtualboxProviderModel describes the provider data model.
type VirtualboxProviderModel struct {
}

func (p *VirtualboxProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "virtualbox"
	resp.Version = p.version
}

func (p *VirtualboxProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{}
}

func (p *VirtualboxProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data VirtualboxProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Configuration values are now available.
	// if data.Endpoint.IsNull() { /* ... */ }

	// Example client configuration for data sources and resources
	client := http.DefaultClient
	resp.DataSourceData = client
	resp.ResourceData = client
}

func (p *VirtualboxProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewVirtualboxVMResource,
	}
}

func (p *VirtualboxProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &VirtualboxProvider{
			version: version,
		}
	}
}
