
resource "null_resource" "sam_build" {
  provisioner "local-exec" {
    working_dir = "./fake_imds_route"
    command = "sam build"
  }
}

resource "null_resource" "sam_package" {
  depends_on = [null_resource.sam_build]

  provisioner "local-exec" {
    working_dir = "./fake_imds_route"
    command = "sam package --resolve-s3"
  }
}

resource "aws_cloudformation_stack" "fake_imds_route" {
  name = "FakeImdsRoute"

  template_body = file("./fake_imds_route/.aws-sam/build/template.yaml")
  on_failure = "DELETE"
  capabilities = ["CAPABILITY_IAM"]
  parameters = {
    FakeImdsInstanceId = aws_instance.imds_server.id
  }
  depends_on = [null_resource.sam_build]
}