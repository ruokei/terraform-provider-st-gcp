data "st-gcp_load_balancer_backend_services" "def" {
  name = "backend-service-name"

  tags = {
    env = "test"
    app = "crond"
  }
}
