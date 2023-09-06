package gcp

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"golang.org/x/net/context"
	"golang.org/x/oauth2/google"
)

// acmeEabResource Present st-gcp_acme_eab resource
type acmeEabResource struct {
	client *gcpClients
}

type acmeEabState struct {
	KeyID      types.String `tfsdk:"key_id"`
	Name       types.String `tfsdk:"name"`
	HmacBase64 types.String `tfsdk:"hmac_base64"`
	CreateAt   types.Int64  `tfsdk:"create_at"` // the unix timestamp of create EAB credential
}

type externalAccountKeyResp struct {
	KeyID     string `json:"keyId"`
	Name      string `json:"name"`
	B64MacKey string `json:"b64MacKey"`
}

// NewAcmeEabResource
func NewAcmeEabResource() resource.Resource {
	return &acmeEabResource{}
}

// Metadata
func (r *acmeEabResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_acme_eab"
}

// Schema
func (r *acmeEabResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Request EAB credential for ACME.",
		Attributes: map[string]schema.Attribute{
			"key_id": &schema.StringAttribute{
				Description: "EAB key ID.",
				Computed:    true,
			},
			"name": &schema.StringAttribute{
				Description: "EAB name.",
				Computed:    true,
			},
			"hmac_base64": &schema.StringAttribute{
				Description: "EAB credential with hmac_base64 format.",
				Computed:    true,
			},
			"create_at": &schema.Int64Attribute{
				Description: "EAB create timestamp.",
				Computed:    true,
			},
		},
	}
}

// Configure
func (r *acmeEabResource) Configure(_ context.Context,
	req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		// this data available on apply stage
		return
	}
	client, ok := req.ProviderData.(*gcpClients)
	if !ok {
		resp.Diagnostics.AddError("req.ProviderData not a gcpClients error", "")
		return
	}
	r.client = client
}

// Create
func (r *acmeEabResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var state acmeEabState
	d := req.Plan.Get(ctx, &state)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		tflog.Error(ctx, "Create req.Plan.Get error")
		return
	}

	if err := createEabCred(ctx, &state, r.client.credentialsJSON, nil); err != nil {
		resp.Diagnostics.AddError("createEabCred error", err.Error())
		return
	}
	resp.State.Set(ctx, &state)
}

// Read
func (r *acmeEabResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
	// Since GCP does not provide an API to get EAB credential, the Read function will not be implemented.
}

// Update
func (r *acmeEabResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var state acmeEabState
	d := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		tflog.Error(ctx, "Update req.State.Get error")
		return
	}

	eabData := externalAccountKeyResp{
		KeyID:     state.KeyID.String(),
		Name:      state.Name.String(),
		B64MacKey: state.HmacBase64.String(),
	}
	if err := createEabCred(ctx, &state, r.client.credentialsJSON, &eabData); err != nil {
		resp.Diagnostics.AddError("createEabCred error", err.Error())
		return
	}
	resp.State.Set(ctx, &state)
}

// Delete
func (r *acmeEabResource) Delete(_ context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	resp.Diagnostics.AddWarning(
		"[Warning] Delete function will do nothing",
		"Since GCP does not provide an API to delete EAB credential, the Delete function will not be implemented.",
	)
}

const (
	maxRetryTimes = 3
	retrySleepMs  = 500
)

type credentialsGcp struct {
	Type                    string `json:"type"`
	ProjectID               string `json:"project_id"`
	PrivateKeyID            string `json:"private_key_id"`
	PrivateKey              string `json:"private_key"`
	ClientEmail             string `json:"client_email"`
	ClientID                string `json:"client_id"`
	AuthURI                 string `json:"auth_uri"`
	TokenURI                string `json:"token_uri"`
	AuthProviderX509CertURL string `json:"auth_provider_x509_cert_url"`
	ClientX509CertURL       string `json:"client_x509_cert_url"`
}

// createEabCred Create a EAB credential.
// nolint:lll
// see: https://cloud.google.com/certificate-manager/docs/reference/public-ca/rest/v1/projects.locations.externalAccountKeys/create
func createEabCred(ctx context.Context, s *acmeEabState, credentialsJSON []byte, old *externalAccountKeyResp) error {
	cred := &credentialsGcp{}
	if err := json.Unmarshal(credentialsJSON, &cred); err != nil {
		return fmt.Errorf("failed to unmarshal GCP credential JSON: %v", err)
	}

	url := "https://www.googleapis.com/auth/cloud-platform"
	conf, err := google.JWTConfigFromJSON(credentialsJSON, url)
	if err != nil {
		return fmt.Errorf("failed to generate JWT config: %v", err)
	}

	var api = fmt.Sprintf(
		"https://publicca.googleapis.com/v1beta1/projects/%s/locations/global/externalAccountKeys",
		cred.ProjectID)
	var postData *bytes.Reader
	if old != nil {
		old.B64MacKey = base64.StdEncoding.Strict().EncodeToString([]byte(old.B64MacKey))
		buf, _ := json.Marshal(old)
		postData = bytes.NewReader(buf)
	}

	var resp *http.Response
	requestFunc := func() error {
		if old != nil {
			resp, err = conf.Client(context.Background()).Post(api, "application/json", postData)
		} else {
			resp, err = conf.Client(context.Background()).Post(api, "application/json", nil)
		}
		defer resp.Body.Close()

		if err != nil {
			errMsg := err.Error()
			tflog.Warn(ctx, "Failed to request API", map[string]interface{}{
				"error": errMsg,
			})
			if strings.Contains(errMsg, "timeout") ||
				strings.Contains(errMsg, " 500 ") ||
				strings.Contains(errMsg, " 504 ") ||
				strings.Contains(errMsg, "DNS") {
				return err
			}
			return &backoff.PermanentError{Err: err}
		}
		return nil
	}
	if err := backoff.Retry(requestFunc, backoff.NewExponentialBackOff()); err != nil {
		return err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("url:" + api + ", error:" + string(body))
	}

	var eab externalAccountKeyResp
	if err = json.Unmarshal(body, &eab); err != nil {
		return fmt.Errorf("failed to unmarshal EAB response: %v", err)
	}
	eabMacKey, err := base64.StdEncoding.DecodeString(eab.B64MacKey)
	if err != nil {
		return fmt.Errorf("failed to base64-decode EAB B64MacKey: %v", err)
	}
	eab.B64MacKey = string(eabMacKey)

	s.KeyID = basetypes.NewStringValue(eab.KeyID)
	s.Name = basetypes.NewStringValue(eab.Name)
	s.HmacBase64 = basetypes.NewStringValue(eab.B64MacKey)
	s.CreateAt = basetypes.NewInt64Value(time.Now().Unix())

	return nil
}
