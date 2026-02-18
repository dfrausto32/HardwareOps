resource "aws_db_subnet_group" "this" {
  name       = "${var.name_prefix}-db-subnet-group"
  subnet_ids = var.private_subnet_ids

  tags = merge(var.tags, {
    Name = "${var.name_prefix}-db-subnet-group"
  })
}

resource "aws_db_parameter_group" "this" {
  name   = "${var.name_prefix}-pg16"
  family = "postgres16"

  parameter {
    name  = "log_statement"
    value = "none"
  }

  tags = merge(var.tags, {
    Name = "${var.name_prefix}-pg16"
  })
}

resource "aws_db_instance" "this" {
  count = var.manage_master_user_password ? 0 : 1

  identifier = "${var.name_prefix}-postgres"

  engine         = "postgres"
  engine_version = var.engine_version
  instance_class = var.instance_class

  db_name  = var.db_name
  username = var.username

  password = var.master_password

  allocated_storage     = var.allocated_storage
  max_allocated_storage = var.max_allocated_storage
  storage_encrypted     = true

  db_subnet_group_name   = aws_db_subnet_group.this.name
  vpc_security_group_ids = [var.db_security_group_id]
  parameter_group_name   = aws_db_parameter_group.this.name
  multi_az               = var.multi_az
  publicly_accessible    = false

  backup_retention_period = var.backup_retention_days
  backup_window           = "03:00-05:00"
  maintenance_window      = "Mon:05:00-Mon:06:00"

  deletion_protection       = var.deletion_protection
  skip_final_snapshot       = var.skip_final_snapshot
  final_snapshot_identifier = var.skip_final_snapshot ? null : "${var.name_prefix}-postgres-final"

  performance_insights_enabled = true
  apply_immediately            = false

  tags = merge(var.tags, {
    Name = "${var.name_prefix}-postgres"
  })
}

resource "aws_db_instance" "managed" {
  count = var.manage_master_user_password ? 1 : 0

  identifier = "${var.name_prefix}-postgres"

  engine         = "postgres"
  engine_version = var.engine_version
  instance_class = var.instance_class

  db_name  = var.db_name
  username = var.username

  manage_master_user_password = true

  allocated_storage     = var.allocated_storage
  max_allocated_storage = var.max_allocated_storage
  storage_encrypted     = true

  db_subnet_group_name   = aws_db_subnet_group.this.name
  vpc_security_group_ids = [var.db_security_group_id]
  parameter_group_name   = aws_db_parameter_group.this.name
  multi_az               = var.multi_az
  publicly_accessible    = false

  backup_retention_period = var.backup_retention_days
  backup_window           = "03:00-05:00"
  maintenance_window      = "Mon:05:00-Mon:06:00"

  deletion_protection       = var.deletion_protection
  skip_final_snapshot       = var.skip_final_snapshot
  final_snapshot_identifier = var.skip_final_snapshot ? null : "${var.name_prefix}-postgres-final"

  performance_insights_enabled = true
  apply_immediately            = false

  tags = merge(var.tags, {
    Name = "${var.name_prefix}-postgres"
  })
}
