output "deploy_role_arn" {
  description = "ARN of the environment deployment role."
  value       = aws_iam_role.deploy.arn
}
