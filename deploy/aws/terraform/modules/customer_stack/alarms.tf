resource "aws_sns_topic" "parcel_alerts" {
  name = "${var.name_prefix}-alerts"
  tags = var.tags
}

resource "aws_sns_topic_subscription" "email" {
  count = var.alarm_sns_email != "" ? 1 : 0

  topic_arn = aws_sns_topic.parcel_alerts.arn
  protocol  = "email"
  endpoint  = var.alarm_sns_email
}

resource "aws_cloudwatch_metric_alarm" "alb_5xx" {
  alarm_name          = "${var.name_prefix}-alb-5xx"
  alarm_description   = "ALB 5xx error count exceeded threshold."
  namespace           = "AWS/ApplicationELB"
  metric_name         = "HTTPCode_ELB_5XX_Count"
  statistic           = "Sum"
  period              = 300
  evaluation_periods  = 1
  threshold           = var.alarm_alb_5xx_threshold
  comparison_operator = "GreaterThanOrEqualToThreshold"
  treat_missing_data  = "notBreaching"

  dimensions = {
    LoadBalancer = regex("(app/.+)", module.alb.alb_arn)[0]
  }

  alarm_actions = [aws_sns_topic.parcel_alerts.arn]
  ok_actions    = [aws_sns_topic.parcel_alerts.arn]

  tags = var.tags
}

resource "aws_cloudwatch_metric_alarm" "alb_unhealthy_hosts" {
  alarm_name          = "${var.name_prefix}-alb-unhealthy-hosts"
  alarm_description   = "ALB unhealthy host count is non-zero."
  namespace           = "AWS/ApplicationELB"
  metric_name         = "UnHealthyHostCount"
  statistic           = "Maximum"
  period              = 120
  evaluation_periods  = 2
  threshold           = 1
  comparison_operator = "GreaterThanOrEqualToThreshold"
  treat_missing_data  = "notBreaching"

  dimensions = {
    LoadBalancer = regex("(app/.+)", module.alb.alb_arn)[0]
    TargetGroup  = regex("(targetgroup/.+)", module.alb.app_target_group_arn)[0]
  }

  alarm_actions = [aws_sns_topic.parcel_alerts.arn]
  ok_actions    = [aws_sns_topic.parcel_alerts.arn]

  tags = var.tags
}

resource "aws_cloudwatch_metric_alarm" "ecs_control_plane_cpu" {
  alarm_name          = "${var.name_prefix}-ecs-cp-cpu"
  alarm_description   = "ECS control-plane CPU utilization exceeded threshold."
  namespace           = "AWS/ECS"
  metric_name         = "CPUUtilization"
  statistic           = "Average"
  period              = 300
  evaluation_periods  = 3
  threshold           = var.alarm_ecs_cpu_threshold
  comparison_operator = "GreaterThanOrEqualToThreshold"
  treat_missing_data  = "notBreaching"

  dimensions = {
    ClusterName = module.ecs.cluster_name
    ServiceName = module.ecs.control_plane_service_name
  }

  alarm_actions = [aws_sns_topic.parcel_alerts.arn]
  ok_actions    = [aws_sns_topic.parcel_alerts.arn]

  tags = var.tags
}

resource "aws_cloudwatch_metric_alarm" "ecs_control_plane_memory" {
  alarm_name          = "${var.name_prefix}-ecs-cp-memory"
  alarm_description   = "ECS control-plane memory utilization exceeded threshold."
  namespace           = "AWS/ECS"
  metric_name         = "MemoryUtilization"
  statistic           = "Average"
  period              = 300
  evaluation_periods  = 3
  threshold           = var.alarm_ecs_memory_threshold
  comparison_operator = "GreaterThanOrEqualToThreshold"
  treat_missing_data  = "notBreaching"

  dimensions = {
    ClusterName = module.ecs.cluster_name
    ServiceName = module.ecs.control_plane_service_name
  }

  alarm_actions = [aws_sns_topic.parcel_alerts.arn]
  ok_actions    = [aws_sns_topic.parcel_alerts.arn]

  tags = var.tags
}

resource "aws_cloudwatch_metric_alarm" "ecs_control_plane_tasks" {
  alarm_name          = "${var.name_prefix}-ecs-cp-tasks"
  alarm_description   = "ECS control-plane running task count dropped below 1 (service down)."
  namespace           = "ECS/ContainerInsights"
  metric_name         = "RunningTaskCount"
  statistic           = "Minimum"
  period              = 60
  evaluation_periods  = 2
  threshold           = 1
  comparison_operator = "LessThanThreshold"
  treat_missing_data  = "breaching"

  dimensions = {
    ClusterName = module.ecs.cluster_name
    ServiceName = module.ecs.control_plane_service_name
  }

  alarm_actions = [aws_sns_topic.parcel_alerts.arn]
  ok_actions    = [aws_sns_topic.parcel_alerts.arn]

  tags = var.tags
}

resource "aws_cloudwatch_metric_alarm" "rds_cpu" {
  alarm_name          = "${var.name_prefix}-rds-cpu"
  alarm_description   = "RDS CPU utilization exceeded threshold."
  namespace           = "AWS/RDS"
  metric_name         = "CPUUtilization"
  statistic           = "Average"
  period              = 300
  evaluation_periods  = 3
  threshold           = var.alarm_rds_cpu_threshold
  comparison_operator = "GreaterThanOrEqualToThreshold"
  treat_missing_data  = "notBreaching"

  dimensions = {
    DBInstanceIdentifier = "${var.name_prefix}-postgres"
  }

  alarm_actions = [aws_sns_topic.parcel_alerts.arn]
  ok_actions    = [aws_sns_topic.parcel_alerts.arn]

  tags = var.tags
}

resource "aws_cloudwatch_metric_alarm" "rds_free_storage" {
  alarm_name          = "${var.name_prefix}-rds-free-storage"
  alarm_description   = "RDS free storage space dropped at or below threshold."
  namespace           = "AWS/RDS"
  metric_name         = "FreeStorageSpace"
  statistic           = "Minimum"
  period              = 300
  evaluation_periods  = 1
  threshold           = var.alarm_rds_free_storage_threshold_gb * 1024 * 1024 * 1024
  comparison_operator = "LessThanOrEqualToThreshold"
  treat_missing_data  = "notBreaching"

  dimensions = {
    DBInstanceIdentifier = "${var.name_prefix}-postgres"
  }

  alarm_actions = [aws_sns_topic.parcel_alerts.arn]
  ok_actions    = [aws_sns_topic.parcel_alerts.arn]

  tags = var.tags
}

resource "aws_cloudwatch_metric_alarm" "rds_connections" {
  alarm_name          = "${var.name_prefix}-rds-connections"
  alarm_description   = "RDS database connections exceeded threshold."
  namespace           = "AWS/RDS"
  metric_name         = "DatabaseConnections"
  statistic           = "Maximum"
  period              = 300
  evaluation_periods  = 2
  threshold           = var.alarm_rds_connections_threshold
  comparison_operator = "GreaterThanOrEqualToThreshold"
  treat_missing_data  = "notBreaching"

  dimensions = {
    DBInstanceIdentifier = "${var.name_prefix}-postgres"
  }

  alarm_actions = [aws_sns_topic.parcel_alerts.arn]
  ok_actions    = [aws_sns_topic.parcel_alerts.arn]

  tags = var.tags
}
