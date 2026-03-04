data "aws_region" "current" {}
data "aws_caller_identity" "current" {}

locals {
  database_url = format(
    "postgres://%s:%s@%s:%d/%s?sslmode=require",
    urlencode(var.db_username),
    urlencode(var.db_master_password),
    module.database.address,
    module.database.port,
    module.database.db_name,
  )

  default_control_plane_env = {
    S3_BUCKET          = module.artifact_store.bucket_name
    S3_REGION          = data.aws_region.current.region
    S3_USE_SSL         = "1"
    S3_ENDPOINT        = "s3.${data.aws_region.current.region}.amazonaws.com"
    DATABASE_URL       = local.database_url
    AUTO_MIGRATE       = "1"
    MIGRATIONS_DIR     = "/app/migrations"
    TRUST_PROXY        = "1"
    TRUST_PROXY_CIDRS  = var.vpc_cidr
    CLIENT_CERT_HEADER = "X-Client-Cert"
  }
  artifact_pull_credentials_secret_id = trimspace(var.artifact_pull_credentials_aws_secret_id != null ? var.artifact_pull_credentials_aws_secret_id : "")
  artifact_pull_credentials_enabled   = local.artifact_pull_credentials_secret_id != ""
  artifact_pull_credentials_secret_arn = startswith(local.artifact_pull_credentials_secret_id, "arn:") ? local.artifact_pull_credentials_secret_id : format(
    "arn:aws:secretsmanager:%s:%s:secret:%s*",
    data.aws_region.current.region,
    data.aws_caller_identity.current.account_id,
    local.artifact_pull_credentials_secret_id,
  )
  artifact_pull_credentials_env = local.artifact_pull_credentials_enabled ? {
    ARTIFACT_PULL_CREDENTIALS_AWS_SECRET_ID = local.artifact_pull_credentials_secret_id
    ARTIFACT_PULL_CREDENTIALS_AWS_REGION    = data.aws_region.current.region
  } : {}

  demo_agents_enabled = var.enable_demo_fleet && var.demo_agent_count > 0 && var.demo_agent_image != null && trimspace(var.demo_agent_image) != ""
  demo_control_plane_env = local.demo_agents_enabled ? {
    CA_CERT_PATH = "/app/demo-certs/ca.crt"
    CA_KEY_PATH  = "/app/demo-certs/ca.key"
  } : {}
  demo_bootstrap_email = var.demo_bootstrap_email != null ? var.demo_bootstrap_email : lookup(var.control_plane_env, "AUTH_BOOTSTRAP_EMAIL", "")
  demo_bootstrap_password = var.demo_bootstrap_password != null ? var.demo_bootstrap_password : lookup(
    var.control_plane_env,
    "AUTH_BOOTSTRAP_PASSWORD",
    "",
  )
  demo_default_agent_env = {
    DEMO_APP_URL            = "https://${var.app_host}"
    DEMO_DEVICES_URL        = "https://${var.devices_host}:8443"
    CHECKIN_INTERVAL_SEC    = tostring(var.demo_agent_checkin_interval_sec)
    DEMO_BOOTSTRAP_EMAIL    = local.demo_bootstrap_email
    DEMO_BOOTSTRAP_PASSWORD = local.demo_bootstrap_password
  }
  effective_demo_agent_env = merge(local.demo_default_agent_env, var.demo_agent_env)
  task_role_managed_policy_arns = compact(concat(
    [
      "arn:aws:iam::aws:policy/AmazonS3ReadOnlyAccess",
      "arn:aws:iam::aws:policy/CloudWatchAgentServerPolicy",
    ],
    local.artifact_pull_credentials_enabled ? [aws_iam_policy.artifact_pull_credentials_read[0].arn] : [],
  ))
}

module "network" {
  source = "../network"

  name_prefix          = var.name_prefix
  vpc_cidr             = var.vpc_cidr
  public_subnet_cidrs  = var.public_subnet_cidrs
  private_subnet_cidrs = var.private_subnet_cidrs
  tags                 = var.tags
}

module "security" {
  source = "../security"

  name_prefix        = var.name_prefix
  vpc_id             = module.network.vpc_id
  vpc_cidr           = module.network.vpc_cidr
  ingress_cidrs      = var.ingress_cidrs
  gateway_port       = var.gateway_container_port
  control_plane_port = var.control_plane_container_port
  tags               = var.tags
}

resource "aws_security_group" "demo_efs" {
  count = local.demo_agents_enabled ? 1 : 0

  name        = "${var.name_prefix}-demo-efs"
  description = "Demo agent EFS security group."
  vpc_id      = module.network.vpc_id

  ingress {
    description     = "NFS from ECS tasks"
    from_port       = 2049
    to_port         = 2049
    protocol        = "tcp"
    security_groups = [module.security.ecs_security_group_id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = merge(var.tags, {
    Name = "${var.name_prefix}-demo-efs"
  })
}

resource "aws_efs_file_system" "demo" {
  count = local.demo_agents_enabled ? 1 : 0

  creation_token = "${var.name_prefix}-demo-agents"
  encrypted      = true

  tags = merge(var.tags, {
    Name = "${var.name_prefix}-demo-agents"
  })
}

resource "aws_efs_mount_target" "demo" {
  for_each = local.demo_agents_enabled ? toset(module.network.private_subnet_ids) : toset([])

  file_system_id  = aws_efs_file_system.demo[0].id
  subnet_id       = each.value
  security_groups = [aws_security_group.demo_efs[0].id]
}

resource "aws_efs_access_point" "demo" {
  count = local.demo_agents_enabled ? var.demo_agent_count : 0

  file_system_id = aws_efs_file_system.demo[0].id

  posix_user {
    uid = 10001
    gid = 10001
  }

  root_directory {
    path = "/agents/agent-${count.index + 1}"

    creation_info {
      owner_gid   = 10001
      owner_uid   = 10001
      permissions = "0755"
    }
  }

  tags = merge(var.tags, {
    Name = "${var.name_prefix}-demo-agent-${count.index + 1}"
  })
}

module "artifact_store" {
  source = "../artifact_store"

  name_prefix       = var.name_prefix
  bucket_name       = var.artifact_bucket_name
  create_kms_key    = var.artifact_store_create_kms_key
  enable_versioning = true
  tags              = var.tags
}

module "database" {
  source = "../database"

  name_prefix                 = var.name_prefix
  db_name                     = var.db_name
  username                    = var.db_username
  master_password             = var.db_master_password
  manage_master_user_password = var.db_manage_master_user_password
  instance_class              = var.db_instance_class
  allocated_storage           = var.db_allocated_storage
  max_allocated_storage       = var.db_max_allocated_storage
  engine_version              = var.db_engine_version
  multi_az                    = var.db_multi_az
  backup_retention_days       = var.db_backup_retention_days
  private_subnet_ids          = module.network.private_subnet_ids
  db_security_group_id        = module.security.db_security_group_id
  deletion_protection         = var.db_deletion_protection
  skip_final_snapshot         = false
  tags                        = var.tags
}

module "alb" {
  source = "../alb"

  name_prefix                = var.name_prefix
  vpc_id                     = module.network.vpc_id
  public_subnet_ids          = module.network.public_subnet_ids
  alb_security_group_id      = module.security.alb_security_group_id
  certificate_arn            = var.acm_certificate_arn
  app_host                   = var.app_host
  devices_host               = var.devices_host
  app_listener_port          = 443
  devices_listener_port      = 8443
  target_port                = var.gateway_container_port
  device_mtls_mode           = var.device_mtls_mode
  device_mtls_bucket         = var.device_mtls_bucket
  device_mtls_key            = var.device_mtls_key
  device_mtls_object_version = var.device_mtls_object_version
  tags                       = var.tags
}

resource "aws_iam_policy" "artifact_pull_credentials_read" {
  count = local.artifact_pull_credentials_enabled ? 1 : 0

  name        = "${var.name_prefix}-artifact-pull-credentials-read"
  description = "Allows ECS tasks to read artifact pull credential secrets."
  path        = "/"
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid    = "ReadArtifactPullCredentials"
        Effect = "Allow"
        Action = [
          "secretsmanager:DescribeSecret",
          "secretsmanager:GetSecretValue",
        ]
        Resource = local.artifact_pull_credentials_secret_arn
      },
    ]
  })
  tags = var.tags
}

module "ecs" {
  source = "../ecs"

  name_prefix                     = var.name_prefix
  vpc_id                          = module.network.vpc_id
  private_subnet_ids              = module.network.private_subnet_ids
  ecs_security_group_id           = module.security.ecs_security_group_id
  app_target_group_arn            = module.alb.app_target_group_arn
  devices_target_group_arn        = module.alb.devices_target_group_arn
  control_plane_image             = var.control_plane_image
  gateway_image                   = var.gateway_image
  control_plane_container_port    = var.control_plane_container_port
  gateway_container_port          = var.gateway_container_port
  default_dns_resolver            = cidrhost(var.vpc_cidr, 2)
  control_plane_desired_count     = var.control_plane_desired_count
  gateway_desired_count           = var.gateway_desired_count
  control_plane_env               = merge(local.default_control_plane_env, local.demo_control_plane_env, local.artifact_pull_credentials_env, var.control_plane_env)
  gateway_env                     = var.gateway_env
  control_plane_secret_arns       = var.control_plane_secret_arns
  gateway_secret_arns             = var.gateway_secret_arns
  enable_demo_agents              = local.demo_agents_enabled
  demo_agent_count                = var.demo_agent_count
  demo_agent_image                = var.demo_agent_image
  demo_agent_cpu                  = var.demo_agent_cpu
  demo_agent_memory               = var.demo_agent_memory
  demo_agent_env                  = local.effective_demo_agent_env
  demo_agent_efs_file_system_id   = local.demo_agents_enabled ? aws_efs_file_system.demo[0].id : null
  demo_agent_efs_access_point_ids = local.demo_agents_enabled ? aws_efs_access_point.demo[*].id : []
  task_role_managed_policy_arns   = local.task_role_managed_policy_arns
  tags                            = var.tags

  depends_on = [aws_efs_mount_target.demo]
}

resource "aws_route53_record" "app" {
  count = var.create_dns_records ? 1 : 0

  zone_id = var.route53_zone_id
  name    = var.app_host
  type    = "A"

  alias {
    name                   = module.alb.alb_dns_name
    zone_id                = module.alb.alb_zone_id
    evaluate_target_health = true
  }
}

resource "aws_route53_record" "devices" {
  count = var.create_dns_records ? 1 : 0

  zone_id = var.route53_zone_id
  name    = var.devices_host
  type    = "A"

  alias {
    name                   = module.alb.alb_dns_name
    zone_id                = module.alb.alb_zone_id
    evaluate_target_health = true
  }
}
