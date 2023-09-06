package gcp

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/mitchellh/go-homedir"
	googleComputeClient "google.golang.org/api/compute/v1"
	"google.golang.org/api/option"
)

type gcpClients struct {
	project         string
	credentialsJSON []byte
	computeClient   *googleComputeClient.Service
}

// Ensure the implementation satisfies the expected interfaces
var (
	_ provider.Provider = &googleCloudProvider{}
)

// New is a helper function to simplify provider server
func New() provider.Provider {
	return &googleCloudProvider{}
}

type googleCloudProvider struct{}

type googleCloudProviderModel struct {
	Project     types.String `tfsdk:"project"`
	Credentials types.String `tfsdk:"credentials"`
}

// Metadata returns the provider type name.
func (p *googleCloudProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "st-gcp"
}

// Schema defines the provider-level schema for configuration data.
func (p *googleCloudProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The GCP provider is used to interact with the many resources " +
			"supported by GCP. The provider needs to be configured with the proper " +
			"credentials before it can be used.",
		Attributes: map[string]schema.Attribute{
			"project": schema.StringAttribute{
				Description: "Project Name for Google Cloud API. May also be provided " +
					"via GOOGLE_PROJECT environment variable.",
				Optional: true,
			},
			"credentials": schema.StringAttribute{
				Description: "Either the path to or the contents of a service account " +
					"key file in JSON format for Google Cloud API. May also be " +
					"provided via GOOGLE_CREDENTIALS environment variable environment " +
					"variable, or generate a service account key file and set the " +
					"GOOGLE_APPLICATION_CREDENTIALS environment variable to the " +
					"path of the JSON file.",
				Optional:  true,
				Sensitive: true,
			},
		},
	}
}

// Configure prepares a GCP API client for data sources and resources.
// nolint:lll
func (p *googleCloudProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config googleCloudProviderModel
	diags := req.Config.Get(ctx, &config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	// If practitioner provided a configuration value for any of the
	// attributes, it must be a known value.
	p.checkConfig(&config, resp)
	if resp.Diagnostics.HasError() {
		return
	}
	// Default values to environment variables, but override
	// with Terraform configuration value if set.
	var project, credential string
	if !config.Project.IsNull() {
		project = config.Project.ValueString()
	} else {
		project = os.Getenv("GOOGLE_PROJECT")
	}

	if !config.Credentials.IsNull() {
		credential = config.Credentials.ValueString()
	} else {
		credential = os.Getenv("GOOGLE_CREDENTIALS")
		if credential == "" {
			credential = os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
		}
	}

	// If any of the expected configuration are missing, return
	// errors with provider-specific guidance.
	p.checkField(project, resp, credential)
	if resp.Diagnostics.HasError() {
		return
	}

	// if this is a path and we can stat it, assume it's file
	credentialsContent := p.loadFromFile(resp, credential)
	if credentialsContent == nil {
		return
	}
	googleClientOption := option.WithCredentialsJSON(credentialsContent)
	computeService, err := googleComputeClient.NewService(ctx, googleClientOption)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to initialize Google Cloud client",
			"Please make sure the credentials is valid.\n"+
				"Additional error message: "+err.Error(),
		)
	}
	if resp.Diagnostics.HasError() {
		return
	}
	clients := gcpClients{
		project:         project,
		credentialsJSON: credentialsContent,
		computeClient:   computeService,
	}
	resp.DataSourceData = &clients
	resp.ResourceData = &clients
}

// nolint:lll
func (*googleCloudProvider) loadFromFile(resp *provider.ConfigureResponse, credential string) []byte {
	/*
		Check whether the credentials is a file as it support either the path to
		or the contents of a service account key file in JSON format.
		reference:
		- https://github.com/hashicorp/terraform-provider-google/blob/80f6dd2fcc1c209ed2b066d9b758db2e34145368/google/path_or_contents.go
	*/
	credentialAbsPath := credential
	if credential[0:1] == "~" {
		var err error
		credentialAbsPath, err = homedir.Expand(credential)
		if err != nil {
			resp.Diagnostics.AddError(
				"[INTERNAL ERROR] Failed to expand homedir of credentials file",
				err.Error(),
			)
			return nil
		}
	}

	var credentialContent []byte
	if _, err := os.Stat(credentialAbsPath); err == nil {
		credentialContent, err = os.ReadFile(credentialAbsPath)
		if err != nil {
			resp.Diagnostics.AddError(
				"[INTERNAL ERROR] Failed to read credentials file",
				err.Error(),
			)
			return nil
		}
	} else {
		credentialContent = []byte(credential)
	}
	return credentialContent
}

func (*googleCloudProvider) checkField(project string, resp *provider.ConfigureResponse, credentials string) {
	if project == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("project"),
			"Missing Google Cloud API project",
			"The provider cannot create the Google Cloud API client as there is a "+
				"missing or empty value for the Google Cloud API project. Set the "+
				"project value in the configuration or use the GOOGLE_PROJECT "+
				"environment variable. If either is already set, ensure the value "+
				"is not empty.",
		)
	}

	if credentials == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("credentials"),
			"Missing Google Cloud API credentials",
			"The provider cannot create the Google Cloud API client as there is a "+
				"missing or empty value for the Google Cloud API credential. Set the "+
				"credential value in the configuration or use the GOOGLE_CREDENTIALS "+
				"environment variable or GOOGLE_APLLICATION_CREDENTIALS environment "+
				"variable. If either is already set, ensure the value is not empty.",
		)
	}
}

func (*googleCloudProvider) checkConfig(config *googleCloudProviderModel, resp *provider.ConfigureResponse) {
	if config.Project.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("project"),
			"Unknown Google Cloud project",
			"The provider cannot create the Google Cloud API client as there is "+
				"an unknown configuration value for the Google Cloud project. Set "+
				"the value statically in the configuration, or use the GOOGLE_PROJECT "+
				"environment variable.",
		)
	}

	if config.Credentials.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("credentials"),
			"Unknown Google Cloud credentials",
			"The provider cannot create the Google Cloud API client as there is "+
				"an unknown configuration value for the Google Cloud credentials. "+
				"Set the value statically in the configuration, or use the GOOGLE_CREDENTIALS "+
				"environment variable. Addtionally, generate a service account key "+
				"file and set the GOOGLE_APPLICATION_CREDENTIALS environment variable "+
				"to the path of the JSON file.",
		)
	}
}

// DataSources
func (p *googleCloudProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewLbBackendServicesDataSource,
	}
}

// Resources
func (p *googleCloudProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewAcmeEabResource,
	}
}
