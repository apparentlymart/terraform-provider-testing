module "simple_hello" {
  source = "../"

  input = "hello"
}

data "testing_assertions" "hello" {
  subject = "Simple module with 'hello'"

  equal "result" {
    statement = "returns 'hello'"

    got  = module.simple_hello.result
    want = "hello"
  }
}
