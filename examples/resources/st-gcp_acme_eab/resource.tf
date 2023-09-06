terraform {
  required_providers {
    st-gcp = {
      source  = "myklst/st-gcp"
      version = "~> 0.1"
    }
  }
}

provider "st-gcp" {}

resource "st-gcp_acme_eab" "eab" {
}

output "eab" {
  value = st-gcp_acme_eab.eab
}
