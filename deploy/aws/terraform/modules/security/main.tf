resource "aws_security_group" "alb" {
  name        = "${var.name_prefix}-alb"
  description = "Public ALB security group."
  vpc_id      = var.vpc_id

  ingress {
    description = "HTTPS for app endpoint"
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = var.ingress_cidrs
  }

  ingress {
    description = "HTTPS for devices endpoint"
    from_port   = 8443
    to_port     = 8443
    protocol    = "tcp"
    cidr_blocks = var.ingress_cidrs
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = merge(var.tags, {
    Name = "${var.name_prefix}-alb"
  })
}

resource "aws_security_group" "ecs" {
  name        = "${var.name_prefix}-ecs"
  description = "ECS service security group."
  vpc_id      = var.vpc_id

  ingress {
    description     = "Gateway traffic from ALB"
    from_port       = var.gateway_port
    to_port         = var.gateway_port
    protocol        = "tcp"
    security_groups = [aws_security_group.alb.id]
  }

  ingress {
    description     = "Control-plane traffic from ALB"
    from_port       = var.control_plane_port
    to_port         = var.control_plane_port
    protocol        = "tcp"
    security_groups = [aws_security_group.alb.id]
  }

  ingress {
    description = "Control-plane traffic from ECS tasks"
    from_port   = var.control_plane_port
    to_port     = var.control_plane_port
    protocol    = "tcp"
    self        = true
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = merge(var.tags, {
    Name = "${var.name_prefix}-ecs"
  })
}

resource "aws_security_group" "db" {
  name        = "${var.name_prefix}-db"
  description = "RDS security group."
  vpc_id      = var.vpc_id

  ingress {
    description     = "PostgreSQL from ECS tasks"
    from_port       = var.db_port
    to_port         = var.db_port
    protocol        = "tcp"
    security_groups = [aws_security_group.ecs.id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = [var.vpc_cidr]
  }

  tags = merge(var.tags, {
    Name = "${var.name_prefix}-db"
  })
}
