provider "aws" {
  region = var.aws_region
}

locals {
  name_prefix = "${var.customer_slug}-${var.environment}"
  common_tags = {
    project     = "hardwareops"
    environment = var.environment
    customer    = var.customer_slug
    owner       = var.owner
    managed_by  = "terraform"
  }
}

module "stack" {
  source = "../../modules/customer_stack"

  name_prefix                             = local.name_prefix
  tags                                    = local.common_tags
  vpc_cidr                                = var.vpc_cidr
  public_subnet_cidrs                     = var.public_subnet_cidrs
  private_subnet_cidrs                    = var.private_subnet_cidrs
  ingress_cidrs                           = var.ingress_cidrs
  app_ingress_cidrs                       = var.app_ingress_cidrs
  device_ingress_cidrs                    = var.device_ingress_cidrs
  acm_certificate_arn                     = var.acm_certificate_arn
  route53_zone_id                         = var.route53_zone_id
  create_dns_records                      = var.create_dns_records
  app_host                                = var.app_host
  devices_host                            = var.devices_host
  device_mtls_bucket                      = var.device_mtls_bucket
  device_mtls_key                         = var.device_mtls_key
  device_mtls_object_version              = var.device_mtls_object_version
  device_mtls_mode                        = var.device_mtls_mode
  enable_waf                              = var.enable_waf
  waf_rate_limit                          = var.waf_rate_limit
  waf_managed_rule_groups                 = var.waf_managed_rule_groups
  artifact_bucket_name                    = var.artifact_bucket_name
  db_instance_class                       = var.db_instance_class
  db_multi_az                             = var.db_multi_az
  db_master_password                      = var.db_master_password
  db_manage_master_user_password          = var.db_manage_master_user_password
  control_plane_image                     = var.control_plane_image
  gateway_image                           = var.gateway_image
  control_plane_desired_count             = var.control_plane_desired_count
  gateway_desired_count                   = var.gateway_desired_count
  control_plane_env                       = var.control_plane_env
  artifact_pull_credentials_aws_secret_id = var.artifact_pull_credentials_aws_secret_id
  trusted_signing_keys_aws_secret_id      = var.trusted_signing_keys_aws_secret_id
  secret_kms_key_arns                     = var.secret_kms_key_arns
  gateway_env                             = var.gateway_env
  control_plane_secret_arns               = var.control_plane_secret_arns
  gateway_secret_arns                     = var.gateway_secret_arns
  enable_demo_fleet                       = var.enable_demo_fleet
  demo_agent_count                        = var.demo_agent_count
  demo_agent_image                        = var.demo_agent_image
  demo_agent_cpu                          = var.demo_agent_cpu
  demo_agent_memory                       = var.demo_agent_memory
  demo_agent_checkin_interval_sec         = var.demo_agent_checkin_interval_sec
  demo_agent_env                          = var.demo_agent_env
  demo_bootstrap_email                    = var.demo_bootstrap_email
  demo_bootstrap_password                 = var.demo_bootstrap_password
}
