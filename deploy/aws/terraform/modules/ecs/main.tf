data "aws_region" "current" {}

locals {
  execution_role_arn           = coalesce(var.execution_role_arn, try(aws_iam_role.execution[0].arn, null))
  task_role_arn                = coalesce(var.task_role_arn, try(aws_iam_role.task[0].arn, null))
  control_plane_discovery_name = "control-plane.${aws_service_discovery_private_dns_namespace.this.name}"
  default_gateway_env = {
    CONTROL_PLANE_UPSTREAM = "${local.control_plane_discovery_name}:${var.control_plane_container_port}"
    GATEWAY_TLS            = "0"
    DNS_RESOLVER           = coalesce(var.default_dns_resolver, "169.254.169.253")
  }
  effective_gateway_env = merge(local.default_gateway_env, var.gateway_env)

  gateway_target_groups = compact([
    var.app_target_group_arn,
    var.devices_target_group_arn,
  ])

  demo_agents_enabled = (
    var.enable_demo_agents &&
    var.demo_agent_count > 0 &&
    var.demo_agent_image != null &&
    trimspace(var.demo_agent_image) != ""
  )
  demo_agent_slots = local.demo_agents_enabled ? [for i in range(var.demo_agent_count) : tostring(i + 1)] : []
  demo_agent_slot_map = {
    for slot in local.demo_agent_slots : slot => tonumber(slot)
  }
}

resource "aws_service_discovery_private_dns_namespace" "this" {
  name = "${var.name_prefix}.internal"
  vpc  = var.vpc_id
  tags = var.tags
}

resource "aws_service_discovery_service" "control_plane" {
  name = "control-plane"

  dns_config {
    namespace_id   = aws_service_discovery_private_dns_namespace.this.id
    routing_policy = "MULTIVALUE"
    dns_records {
      ttl  = 10
      type = "A"
    }
  }

  tags = var.tags
}

data "aws_iam_policy_document" "ecs_assume_role" {
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "execution" {
  count = var.execution_role_arn == null ? 1 : 0

  name               = "${var.name_prefix}-ecs-exec"
  assume_role_policy = data.aws_iam_policy_document.ecs_assume_role.json
  tags               = var.tags
}

resource "aws_iam_role_policy_attachment" "execution_default" {
  count = var.execution_role_arn == null ? 1 : 0

  role       = aws_iam_role.execution[0].name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

resource "aws_iam_role" "task" {
  count = var.task_role_arn == null ? 1 : 0

  name               = "${var.name_prefix}-ecs-task"
  assume_role_policy = data.aws_iam_policy_document.ecs_assume_role.json
  tags               = var.tags
}

resource "aws_iam_role_policy_attachment" "task_managed" {
  for_each = var.task_role_arn == null ? toset(var.task_role_managed_policy_arns) : toset([])

  role       = aws_iam_role.task[0].name
  policy_arn = each.value
}

resource "aws_cloudwatch_log_group" "control_plane" {
  name              = "/hardwareops/${var.name_prefix}/control-plane"
  retention_in_days = 30
  tags              = var.tags
}

resource "aws_cloudwatch_log_group" "gateway" {
  name              = "/hardwareops/${var.name_prefix}/gateway"
  retention_in_days = 30
  tags              = var.tags
}

resource "aws_cloudwatch_log_group" "demo_agents" {
  count = local.demo_agents_enabled ? 1 : 0

  name              = "/hardwareops/${var.name_prefix}/demo-agents"
  retention_in_days = 30
  tags              = var.tags
}

resource "aws_ecs_cluster" "this" {
  name = "${var.name_prefix}-cluster"
  tags = var.tags
}

resource "aws_ecs_task_definition" "control_plane" {
  family                   = "${var.name_prefix}-control-plane"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = tostring(var.control_plane_cpu)
  memory                   = tostring(var.control_plane_memory)
  execution_role_arn       = local.execution_role_arn
  task_role_arn            = local.task_role_arn

  container_definitions = jsonencode([
    {
      name      = "control-plane"
      image     = var.control_plane_image
      essential = true
      portMappings = [
        {
          containerPort = var.control_plane_container_port
          hostPort      = var.control_plane_container_port
          protocol      = "tcp"
        }
      ]
      environment = [
        for k, v in var.control_plane_env : {
          name  = k
          value = v
        }
      ]
      secrets = [
        for k, v in var.control_plane_secret_arns : {
          name      = k
          valueFrom = v
        }
      ]
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          awslogs-group         = aws_cloudwatch_log_group.control_plane.name
          awslogs-region        = data.aws_region.current.region
          awslogs-stream-prefix = "ecs"
        }
      }
    }
  ])

  tags = var.tags
}

resource "aws_ecs_task_definition" "gateway" {
  family                   = "${var.name_prefix}-gateway"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = tostring(var.gateway_cpu)
  memory                   = tostring(var.gateway_memory)
  execution_role_arn       = local.execution_role_arn
  task_role_arn            = local.task_role_arn

  container_definitions = jsonencode([
    {
      name      = "gateway"
      image     = var.gateway_image
      essential = true
      portMappings = [
        {
          containerPort = var.gateway_container_port
          hostPort      = var.gateway_container_port
          protocol      = "tcp"
        }
      ]
      environment = [
        for k, v in local.effective_gateway_env : {
          name  = k
          value = v
        }
      ]
      secrets = [
        for k, v in var.gateway_secret_arns : {
          name      = k
          valueFrom = v
        }
      ]
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          awslogs-group         = aws_cloudwatch_log_group.gateway.name
          awslogs-region        = data.aws_region.current.region
          awslogs-stream-prefix = "ecs"
        }
      }
    }
  ])

  tags = var.tags
}

resource "aws_ecs_task_definition" "demo_agent" {
  for_each = local.demo_agent_slot_map

  family                   = "${var.name_prefix}-demo-agent-${each.key}"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = tostring(var.demo_agent_cpu)
  memory                   = tostring(var.demo_agent_memory)
  execution_role_arn       = local.execution_role_arn
  task_role_arn            = local.task_role_arn

  container_definitions = jsonencode([
    {
      name      = "demo-agent"
      image     = var.demo_agent_image
      essential = true
      environment = concat(
        [
          for k, v in var.demo_agent_env : {
            name  = k
            value = v
          }
        ],
        [
          {
            name  = "DEMO_AGENT_SLOT"
            value = each.key
          }
        ]
      )
      mountPoints = [
        {
          sourceVolume  = "demo_data"
          containerPath = "/data"
          readOnly      = false
        }
      ]
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          awslogs-group         = aws_cloudwatch_log_group.demo_agents[0].name
          awslogs-region        = data.aws_region.current.region
          awslogs-stream-prefix = "ecs"
        }
      }
    }
  ])

  volume {
    name = "demo_data"

    efs_volume_configuration {
      file_system_id     = var.demo_agent_efs_file_system_id
      transit_encryption = "ENABLED"

      authorization_config {
        access_point_id = var.demo_agent_efs_access_point_ids[each.value - 1]
        iam             = "DISABLED"
      }
    }
  }

  tags = var.tags
}

resource "aws_ecs_service" "control_plane" {
  name            = "${var.name_prefix}-control-plane"
  cluster         = aws_ecs_cluster.this.id
  task_definition = aws_ecs_task_definition.control_plane.arn
  desired_count   = var.control_plane_desired_count
  launch_type     = "FARGATE"

  enable_execute_command = var.enable_execute_command

  deployment_circuit_breaker {
    enable   = true
    rollback = true
  }

  network_configuration {
    subnets          = var.private_subnet_ids
    security_groups  = [var.ecs_security_group_id]
    assign_public_ip = var.assign_public_ip
  }

  service_registries {
    registry_arn   = aws_service_discovery_service.control_plane.arn
    container_name = "control-plane"
  }

  tags = var.tags
}

resource "aws_ecs_service" "gateway" {
  name            = "${var.name_prefix}-gateway"
  cluster         = aws_ecs_cluster.this.id
  task_definition = aws_ecs_task_definition.gateway.arn
  desired_count   = var.gateway_desired_count
  launch_type     = "FARGATE"

  enable_execute_command            = var.enable_execute_command
  health_check_grace_period_seconds = length(local.gateway_target_groups) > 0 ? 60 : null

  deployment_circuit_breaker {
    enable   = true
    rollback = true
  }

  network_configuration {
    subnets          = var.private_subnet_ids
    security_groups  = [var.ecs_security_group_id]
    assign_public_ip = var.assign_public_ip
  }

  dynamic "load_balancer" {
    for_each = toset(local.gateway_target_groups)
    content {
      target_group_arn = load_balancer.value
      container_name   = "gateway"
      container_port   = var.gateway_container_port
    }
  }

  tags = var.tags
}

resource "aws_ecs_service" "demo_agent" {
  for_each = aws_ecs_task_definition.demo_agent

  name            = "${var.name_prefix}-demo-agent-${each.key}"
  cluster         = aws_ecs_cluster.this.id
  task_definition = each.value.arn
  desired_count   = 1
  launch_type     = "FARGATE"

  enable_execute_command = var.enable_execute_command

  deployment_circuit_breaker {
    enable   = true
    rollback = true
  }

  network_configuration {
    subnets          = var.private_subnet_ids
    security_groups  = [var.ecs_security_group_id]
    assign_public_ip = var.assign_public_ip
  }

  tags = var.tags
}
