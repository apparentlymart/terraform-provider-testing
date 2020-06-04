# `testing_tap` Data Source

`testing_tap` is a special data source that can help with writing integration
tests for reusable Terraform modules by launching a separate program that
emits test results using
[the Test Anything Protocol](https://testanything.org/), or _TAP_.

TAP is a relatively straightforward line-oriented protocol that is intended to
be easy to produce in a variety of different programming languages, including
shell scripts.

Although the `testing_assertions` data source can represent various simple
test assertions, `testing_tap` can be useful in more complex cases where it's
more convenient to write the tests in another programming language.

## Example Usage

```hcl
data "testing_tap" "hello" {
  program = ["bash", "${path.module}/test.sh", module.mut.string_result]
}
```

## Argument Reference

`testing_tap` accepts the following arguments:

* `program` (list of strings) - the program to run, expressed as a list of
  arguments in the Unix "argv" style where the executable program is the first
  element and any subsequent elements are individual arguments to that program.

* `environment` (map of strings) - environment variables to set for the child
  test program, where map keys are the environment variable names to set.

If the test program reports any test failures (using "not ok" reports) then
`testing_tap` will report these as error diagnostics. Otherwise, the data
source will succeed.

Note that both command line arguments and environment variables are required
to be strings, so if you intend to send other data types to the test program
you will need to serialize them to string values first. For collection and
structural types, consider using
[the `jsonencode` function](https://www.terraform.io/docs/configuration/functions/jsonencode.html)
to produce a JSON representation of the value which the test program can then
parse using a JSON parser available for its implementation language.

**Note:** The JSON type system is a subset of Terraform's, so serializing to
JSON will not retain all of the exact types Terraform understands. For example,
a test program receiving a JSON object will be unable to determine whether
it was produced from a map value an object value in the Terraform language.
If you wish to perform exact type checking, use `testing_assertions` to perform
those checks within the Terraform language itself.

## Attribute Reference

Because `testing_tap` is designed to either succeed or fail depending
on the testing outcome, unlike "normal" data sources it does not produce any
result attributes.
