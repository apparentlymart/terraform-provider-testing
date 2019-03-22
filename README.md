# Terraform Testing Provider

This provider is an experiment in writing simple tests for your reusable
Terraform modules using Terraform itself.

It's currently very much a prototype and subject to change at any time as we
continue to explore the use-cases and possible approaches.

This is not a HashiCorp project.

## Usage

This provider hopes to establish a new pattern of writing "test modules"
that wrap the module(s) under test by instantiating the target modules and
then making assertions about their results, possibly using data sources from
other Terraform providers to gather data to assert against.

For example, a repository containing a single reusable module might have
a structure like this:

```
variables.tf
main.tf
outputs.tf
test/
  test.tf
```

The root directory is the module that is intended to be used by others. The
`test` directory contains another module that instantiates the module from
the root directory and then uses this provider's `testing_assertions` data
source, possibly in conjunction with other provider data sources, to produce
an error if any of the assertions prove untrue:

```hcl
resource "random_id" "test" {
  byte_length = 16
}

module "mut" { # mut == "module under test"
  source = "../"

  name_prefix = "example-test-${random_id.test.hex}"
  # ... and any other input variables the main module might expect
}

data "http" "terraform_disco" {
  url = "${module.mut.base_url}/.well-known/terraform.json"
}

data "testing_assertions" "terraform_disco" {
  subject = "Terraform discovery document"

  equal "contents" {
    statement = "has the expected content"

    got  = jsondecode(data.http.terraform_disco.body)
    want = {
      "modules.v1": "${module.mut.base_url}/modules/v1"
    }
  }
  equal "content_type" {
    statement = "has JSON content type"

    got  = data.http.terraform_disco.response_headers["content-type"]
    want = "application/json"
  }
}
```

With the `testing` provider installed, you can run `terraform apply` in the
`test` subdirectory to instantiate the module under test (which in this case
seems to be describing a [Terraform module registry](https://www.terraform.io/docs/registry/api.html)),
retrieve the data at a URL under its returned base URL, and then make some
assertions about it.

If the module were buggy it might cause this "discovery" document to be served
with the wrong `Content-Type` header value, in which case the `testing_assertions`
data source would return an error like this:

```
Error: Test failure

  on test.tf line 30, in data "testing_assertions" "terraform_disco":
  13:     got  = data.http.terraform_disco.response_headers["content-type"]

Assertion failed: Terraform discovery document has JSON content type
  Want: "application/json"
  Got:  "text/plain; charset=utf-8"
```

You can then make changes to the main module to fix this bug and run
`terraform apply` again, applying any changes you made (but with no need to
wait for the recreation of anything that was already created correctly on the
first run) and re-running the assertion checks.

Once the apply run completes successfully, you can clean up the temporary test
infrastructure using `terraform destroy` as normal.

## Requirements

This provider requires Terraform v0.12 or later. It is not compatible with
Terraform v0.10 or v0.11.

## External Test Programs

Lots of simple test assertions can be implemented by combining existing Terraform
provider data sources with the `testing_assertions` data source as above.
Sometimes though, it's more convenient to express you test cases in an
imperative program that might have some side-effects of its own.

To support this, the provider also offers a `testing_tap` data source which
runs an external program and interprets its output as the line-oriented
[Test Anything Protocol](https://testanything.org/). This protocol is easy
to generate from any language that can write to stdout -- including shell scripts!
-- and provides a lightweight interface between the test program and this
provider.

```hcl
module "tap_hello" {
  source = "../"

  input = "hello"
}

data "testing_tap" "hello" {
  program = ["bash", "${path.module}/test.sh", module.tap_hello.result]
}
```

If the test program reports any test failures (using "not ok" reports) then
the provider will report these as error diagnostics.
