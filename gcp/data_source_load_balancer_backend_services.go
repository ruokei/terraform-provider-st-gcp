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
	"google.golang.org/api/option"
)

var (
	_ datasource.DataSource              = &LbBackendServicesDataSource{}
	_ datasource.DataSourceWithConfigure = &LbBackendServicesDataSource{}
)

// NewLbBackendServicesDataSource
func NewLbBackendServicesDataSource() datasource.DataSource {
	return &LbBackendServicesDataSource{}
}

// LbBackendServicesDataSource
type LbBackendServicesDataSource struct {
	project string
	client  *googleComputeClient.Service
}

// LbBackendServicesDataSourceModel
type LbBackendServicesDataSourceModel struct {
	ClientConfig *clientConfig                 `tfsdk:"client_config"`
	Name         types.String                  `tfsdk:"name"`
	Tags         types.Map                     `tfsdk:"tags"`
	Items        []*lbBackendServicesItemModel `tfsdk:"items"`
}

type lbBackendServicesItemModel struct {
	ID   types.Int64 `tfsdk:"id"`
	Tags types.Map   `tfsdk:"tags"`
}

type clientConfig struct {
	Project     types.String `tfsdk:"project"`
	Credentials types.String `tfsdk:"credentials"`
}

// Metadata returns the data source backend services type name.
func (d *LbBackendServicesDataSource) Metadata(_ context.Context,
	req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_load_balancer_backend_services"
}

// Schema defines the schema for the backend services data source .
func (d *LbBackendServicesDataSource) Schema(_ context.Context,
	_ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
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
		Blocks: map[string]schema.Block{
			"client_config": schema.SingleNestedBlock{
				Description: "Config to override default client created in Provider. " +
					"This block will not be recorded in state file.",
				Attributes: map[string]schema.Attribute{
					"project": schema.StringAttribute{
						Description: "Project Name for Google Cloud API. Default " +
							"to use project configured in the provider.",
						Optional: true,
					},
					"credentials": schema.StringAttribute{
						Description: "The credentials of service account in JSON format " +
							" Default to use credentials configured in the provider.",
						Optional:  true,
						Sensitive: true,
					},
				},
			},
		},
	}
}

// Configure adds the provider configured client to the data source.
func (d *LbBackendServicesDataSource) Configure(_ context.Context,
	req datasource.ConfigureRequest, _ *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	d.project = req.ProviderData.(*gcpClients).project
	d.client = req.ProviderData.(*gcpClients).computeClient
}

// Read backend services data source information
func (d *LbBackendServicesDataSource) Read(ctx context.Context,
	req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var plan *LbBackendServicesDataSourceModel
	diags := req.Config.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.ClientConfig == nil {
		plan.ClientConfig = &clientConfig{}
	}

	initClient := false
	project := plan.ClientConfig.Project.ValueString()
	credentials := plan.ClientConfig.Credentials.ValueString()
	if project != "" || credentials != "" {
		initClient = true
	}

	if initClient {
		err := d.initClient(ctx, project, credentials, resp)
		if err != nil {
			return
		}
	}

	// Initialize input into state
	state := &LbBackendServicesDataSourceModel{}
	state.Items = []*lbBackendServicesItemModel{}

	// Get list of backend services
	// if backendService.Description != "" {
	// Convert service description (tags) to Map
	// Convert Map to types.Map
	// If the input name is not empty, then compare the input name with
	// the Google backend service name. Continue to next backend service
	// if the name is not matched.
	// If the input tag is not empty, then compare the input tag with
	// the Google backend service tags (extracted from description).
	// Tags comparison.
	// If the key is not found or the tag value is not matched,
	// then break the checking and continue to next backend service.
	// }
	err := d.runBackendServices(ctx, resp, plan, state)
	if err != nil {
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

func (d *LbBackendServicesDataSource) runBackendServices(ctx context.Context,
	resp *datasource.ReadResponse, plan *LbBackendServicesDataSourceModel,
	state *LbBackendServicesDataSourceModel) error {
	responseByList := d.client.BackendServices.List(d.project)
	if err := responseByList.Pages(
		ctx,
		func(page *googleComputeClient.BackendServiceList) error {
			for _, backendService := range page.Items {

				slbTags := make(map[string]attr.Value)
				slbTagsTfType := types.MapNull(types.StringType)

				if backendService.Description != "" {
					tags := strings.Split(backendService.Description, "|")
					for _, tag := range tags {
						t := strings.Split(tag, ":")
						slbTags[t[0]] = types.StringValue(t[1])
					}

					var convertMapDiags diag.Diagnostics
					slbTagsTfType, convertMapDiags = types.MapValue(types.StringType, slbTags)
					resp.Diagnostics.Append(convertMapDiags...)
					if resp.Diagnostics.HasError() {
						return fmt.Errorf("[INTERNAL ERROR] Failed to convert description to tags")
					}
				}

				serviceItem := &lbBackendServicesItemModel{
					ID:   types.Int64Value(int64(backendService.Id)),
					Tags: slbTagsTfType,
				}

				if !(plan.Name.IsUnknown() || plan.Name.IsNull()) && plan.Name.ValueString() != backendService.Name {
					continue
				}

				if !(plan.Tags.IsUnknown() || plan.Tags.IsNull()) {

					matched := true
					goInputMap := plan.Tags.Elements()
					for inputKey, inputValue := range goInputMap {
						value, ok := slbTags[inputKey]

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

			return nil
		},
	); err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to list load balancer backend services.",
			err.Error(),
		)
		return err
	}
	return nil
}

func (d *LbBackendServicesDataSource) initClient(ctx context.Context,
	project string, credentials string, resp *datasource.ReadResponse) error {
	if project != "" {
		d.project = project
	}
	if credentials != "" {
		googleClientOption := option.WithCredentialsJSON([]byte(credentials))
		var err error
		d.client, err = googleComputeClient.NewService(ctx, googleClientOption)
		if err != nil {
			resp.Diagnostics.AddError(
				"[API ERROR] Failed to Reinitialize Google Cloud client",
				"Please make sure the credentials is valid.\n"+
					"Additional error message: "+err.Error(),
			)
			return err
		}
	}
	return nil
}
