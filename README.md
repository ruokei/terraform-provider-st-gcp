Terraform Custom Provider for Google Cloud Platform
===================================================

This Terraform custom provider is designed for own use case scenario.

Supported Versions
------------------

| Terraform version | minimum provider version |maxmimum provider version
| ---- | ---- | ----|
| >= 1.3.x	| 0.1.0	| latest |

Requirements
------------

-	[Terraform](https://www.terraform.io/downloads.html) 1.3.x
-	[Go](https://golang.org/doc/install) 1.19 (to build the provider plugin)

Local Installation
------------------

1. Run `make install-local-custom-provider` to install the provider under ~/.terraform.d/plugins.

2. The provider source should be change to the path that configured in the *Makefile*:

    ```
    terraform {
      required_providers {
        st-gcp = {
          source = "example.local/myklst/st-gcp"
        }
      }
    }

    provider "st-gcp" {}
    ```

Why Custom Provider
-------------------

This custom provider exists due to some of the resources and data sources in the
official GCP Terraform provider may not fulfill the requirements of some scenario.
The reason behind every resources and data sources are stated as below:

### Data Sources

- **st-gcp_load_balancer_backend_services**

  - The load balancer backend services on Google Cloud do not support tagging, therefore
    backend service's description is used as tags with the format
    `TagKey1:TagValue1|TagKey2:TagValue2`, where the character `|` is used as string
    delimiter. Output will also convert description string to map if all are matched.

  - Added client_config block to allow overriding the Provider configuration.

### Resource

- **st-gcp_acme_eab**

  To create [EAB](https://docs.digicert.com/en/trust-lifecycle-manager/integration-guides/third-party-acme-integration/acme-external-account-binding--eab-.html) credential for ACME protocol.

  > ACME EAB credentials
  >
  > The ACME protocol (RFC 8555) defines an external account binding (EAB) field
  > that ACME clients can use to access a specific account on the certificate
  > authority (CA).
  >
  > DigiCert​​®​​’s ACME implementation uses the EAB field to identify both your
  > DigiCert​​®​​ Trust Lifecycle Manager account and a specific certificate profile
  > there.
  >
  > Your ACME client must send the following EAB credentials to request certificates:
  >
  > * **Key identifier (KID)**
  >
  >    Identifies your DigiCert ONE account and the automation profile for certificate issuance.
  >
  > * **HMAC key**
  >
  >    Used to encrypt and authenticate your account key during automation events.

  See:
    - [ACME EAB - What Is It, and How Do We Use It at Smallstep?](https://smallstep.com/blog/acme-eab-overview/)
    - [Google OAuth2 Doc](https://developers.google.com/identity/protocols/oauth2/service-account)
    - [Google Public CA Doc](https://cloud.google.com/certificate-manager/docs/reference/rest/v1beta1/projects.locations.externalAccountKeys/create)
    - [example: examples/resources/st-gcp_acme_eab/resource.tf](examples/resources/st-gcp_acme_eab/resource.tf)
    - Work with [Terraform ACME Certificate and Account Provider](https://registry.terraform.io/providers/vancluever/acme/latest/docs)

References
----------

- Terraform website: https://www.terraform.io
- Terraform Plugin Framework: https://developer.hashicorp.com/terraform/tutorials/providers-plugin-framework
- GCP official Terraform provider: https://github.com/hashicorp/terraform-provider-google
