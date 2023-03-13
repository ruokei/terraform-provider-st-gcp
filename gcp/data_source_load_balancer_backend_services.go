package gcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	googleComputeClient "google.golang.org/api/compute/v1"
)

var (
	_ datasource.DataSource              = &LbBackendServicesDataSource{}
	_ datasource.DataSourceWithConfigure = &LbBackendServicesDataSource{}
)

func NewLbBackendServicesDataSource() datasource.DataSource {
	return &LbBackendServicesDataSource{}
}

type LbBackendServicesDataSource struct {
	project string
	client  *googleComputeClient.Service
}

type LbBackendServicesDataSourceModel struct {
	Name  types.String                `tfsdk:"name"`
	Tags  types.Map                   `tfsdk:"tags"`
	Items []*lbBackendServicesItemModel `tfsdk:"items"`
}

type lbBackendServicesItemModel struct {
	Id   types.Int64 `tfsdk:"id"`
	Tags types.Map   `tfsdk:"tags"`
}

// Metadata returns the data source backend services type name.
func (d *LbBackendServicesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_load_balancer_backend_services"
}

// Schema defines the schema for the backend services data source .
func (d *LbBackendServicesDataSource) Schema(_ context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "This data source provides the load balancer backend services on Google Cloud.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Description: "Name of backend service to be filtered.",
				Optional:    true,
			},
			"tags": schema.MapAttribute{
				Description: "Tags of backend service to be filtered.",
				ElementType: types.StringType,
				Optional:    true,
			},
			"items": schema.ListNestedAttribute{
				Description: "List of queried load balancer backend services.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.Int64Attribute{
							Description: "ID of backend service.",
							Computed:    true,
						},
						"tags": schema.MapAttribute{
							Description: "Tags of backend service.",
							ElementType: types.StringType,
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

// Configure adds the provider configured client to the data source.
func (d *LbBackendServicesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	d.project = req.ProviderData.(gcpClients).project
	d.client = req.ProviderData.(gcpClients).computeClient
}

// Read backend services data source information
func (d *LbBackendServicesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var plan *LbBackendServicesDataSourceModel
	diags := req.Config.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Initialize input into state
	state := &LbBackendServicesDataSourceModel{}
	state.Items = []*lbBackendServicesItemModel{}

	// Get list of backend services
	responseByList := d.client.BackendServices.List(d.project)
	if err := responseByList.Pages(
		ctx,
		func(page *googleComputeClient.BackendServiceList) error {
			for _, backendService := range page.Items {
				// if backendService.Description != "" {
				// Convert service description (tags) to Map

				slbTags := make(map[string]attr.Value)
				slbTagsTfType := types.MapNull(types.StringType)

				if backendService.Description != "" {
					tags := strings.Split(backendService.Description, "|")
					for _, tag := range tags {
						t := strings.Split(tag, ":")
						slbTags[t[0]] = types.StringValue(t[1])
					}
					// Convert Map to types.Map
					var convertMapDiags diag.Diagnostics
					slbTagsTfType, convertMapDiags = types.MapValue(types.StringType, slbTags)
					resp.Diagnostics.Append(convertMapDiags...)
					if resp.Diagnostics.HasError() {
						return fmt.Errorf("[INTERNAL ERROR] Failed to convert description to tags")
					}
				}

				serviceItem := &lbBackendServicesItemModel{
					Id:   types.Int64Value(int64(backendService.Id)),
					Tags: slbTagsTfType,
				}

				// If the input name is not empty, then compare the input name with
				// the Google backend service name. Continue to next backend service
				// if the name is not matched.
				if !(plan.Name.IsUnknown() || plan.Name.IsNull()) && plan.Name.ValueString() != backendService.Name {
					continue
				}

				// If the input tag is not empty, then compare the input tag with
				// the Google backend service tags (extracted from description).
				if !(plan.Tags.IsUnknown() || plan.Tags.IsNull()) {
					// Tags comparison.
					matched := true
					goInputMap := plan.Tags.Elements()
					for inputKey, inputValue := range goInputMap {
						value, ok := slbTags[inputKey]
						// If the key is not found or the tag value is not matched,
						// then break the checking and continue to next backend service.
						if !ok || value != inputValue {
							matched = false
							break
						}
					}
					if !matched {
						continue
					}
				}

				state.Items = append(state.Items, serviceItem)
			}
			// }
			return nil
		},
	); err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to list load balancer backend services.",
			err.Error(),
		)
		return
	}

	state.Name = plan.Name
	state.Tags = plan.Tags

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}
