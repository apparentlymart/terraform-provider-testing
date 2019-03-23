module "tap_hello" {
  source = "../"

  input = "hello"
}

data "testing_tap" "hello" {
  program = ["bash", "${path.module}/test.sh", module.tap_hello.result]

  environment = {
    FOO = "bar"
  }
}
