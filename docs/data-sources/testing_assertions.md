# `testing_assertions` Data Source

`testing_assertions` is a special data source that can compare expected results
with actual results and return errors in case of any mismatch. It's intended
to help with writing simple integration tests for reusable Terraform modules.

## Example Usage

```hcl
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

## Argument Reference

A `testing_assertions` starts by describing what is being tested:

* `subject` (string) - a natural language noun phrase describing what object
  we are making assertions about.
  
    When working in English, this should usually be a sequence of words that
    could have "...is the object we are testing" appended to it and produce a
    grammatical sentence, such as "Terraform discovery document is the object
    we are testing" for the above example.

An assertions block can then make multiple assertions using nested `equal`
or `check` blocks. All of the assertions inside a `testing_assertions` must
pass in order for the data source to succeed.

Each of these blocks has a label that is intended to serve
as a machine-friendly unique identifier for the test, like `"contents"` and
`"content_type"` in the above example. That is not currently used in any
significant way, but may in future be used by a custom test-executing harness
to produce machine-readable test output that has stable identifiers for each of
the tests.

### The assertion statement

Both `equal` and `check` blocks have one nested argument in common:

* `statement` (string) - a natural language description of what the assertion
  is aiming to verify.

    When working in English, this should usually be a sequence of words that
    could complete a sentence starting with "The object we are testing ...",
    such as "The object we are testing has the expected content" in the
    above example.

    When producing error messages, the provider will concatenate the top-level
    subject and the assertion-level statement in the hope of producing a
    grammatical error message, such as
    "Terraform discovery document has JSON data type" for the second `equal`
    block in the above example.

### `equal` blocks

An `equal` block makes an assertion by comparing a returned value against an
expected value and producing an error if they are not equal.

In addition to the common `statement` argument described above, an `equal`
block expects the following additional nested arguments:

* `want` (any type) - a value describing the outcome that the assertion expects.
* `got` (any type) - the value that the module under test actually produced.

An `equal` assertion will verify that the `got` value is equal to the `want`
value. Value equality also requires _type_ equality, so when working with
complex types (lists, maps, etc) it will often be necessary to use explicit
type conversions in the `want` expression to ensure that it is of the same
type that the module is expected to produce. For example:

```hcl
  # Without the "tomap" call, the { ... } syntax produces an object-typed value,
  # which will not compare equal to a map. Calling "tomap" asks Terraform to
  # produce a map derived from the object, which in this case would be a map of
  # strings.
  want = tomap({
    foo = "bar"
    baz = "boop"
  })
```

### `check` blocks

A `check` block makes an assertion by evaluating an expression that should
produce a boolean result, returning `true` if the assertion holds and `false`
if it does not.

In addition to the common `statement` argument described above, a `check` block
expects the following additional nested argument:

* `expect` (boolean) - an expression that will return `true` if the assertion
  holds, and `false` if not.

The `expect` expression should be written such that it will not itself produce
any errors, because any errors during `expect` evaluation will mask the actual
assertion test result. However, assertions in the Terraform language are often
expressed most concisely by evaluating an expression that produces an error
when the assertion does not hold, so Terraform offers
[the `can` function](https://www.terraform.io/docs/configuration/functions/can.html)
to transform a "success vs. error" result into a "`true` vs. `false`" result
as the `expect` argument requires.

For example, to verify that a given return string matches a regular expression
pattern:

```hcl
  expect = can(regex("^https:", module.mut.base_url))
```

## Attribute Reference

Because `testing_assertions` is designed to either succeed or fail depending
on the testing outcome, unlike "normal" data sources it does not produce any
result attributes.
