variable "name_prefix" {
  description = "Prefix used for network resource names."
  type        = string
}

variable "vpc_cidr" {
  description = "CIDR block for the VPC."
  type        = string
}

variable "public_subnet_cidrs" {
  description = "Public subnet CIDRs keyed by AZ name (for example us-east-1a)."
  type        = map(string)
}

variable "private_subnet_cidrs" {
  description = "Private subnet CIDRs keyed by AZ name (for example us-east-1a)."
  type        = map(string)
}

variable "tags" {
  description = "Common tags applied to all resources."
  type        = map(string)
  default     = {}
}
