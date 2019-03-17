module "simple_howdy" {
  source = "../"

  input = "howdy"
}

data "testing_assertions" "howdy" {
  subject = "Simple module with 'howdy'"

  equal "result" {
    statement = "returns 'howdy'"

    got  = module.simple_howdy.result
    want = "howdy"
  }
}
