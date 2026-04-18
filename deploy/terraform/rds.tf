resource "aws_db_subnet_group" "argus" {
  name       = "argus"
  subnet_ids = aws_subnet.private[*].id
  tags       = { Name = "argus" }
}

resource "aws_db_instance" "argus" {
  identifier        = "argus"
  engine            = "postgres"
  engine_version    = "15"
  instance_class    = "db.t3.micro"
  allocated_storage = 20
  storage_type      = "gp2"

  db_name  = "argus"
  username = "argus"
  password = var.db_password

  db_subnet_group_name   = aws_db_subnet_group.argus.name
  vpc_security_group_ids = [aws_security_group.rds.id]
  publicly_accessible    = false

  backup_retention_period   = 7
  skip_final_snapshot       = false
  final_snapshot_identifier = "argus-final"

  tags = { Name = "argus" }
}
