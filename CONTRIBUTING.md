# Contributing

Thank you for your interest in contributing to the Terraform Provider for StorageGRID! 🎉

We appreciate that you want to take the time to contribute! Please follow these steps before submitting your PR.

# Creating a Pull Request

1. Please search [existing issues](https://github.com/NetApp/terraform-provider-netapp-storagegrid/issues) to determine if an issue already exists for what you intend to contribute.
2. If the issue does not exist, [create a new one](https://github.com/NetApp/terraform-provider-netapp-storagegrid/issues/new/choose) that explains the bug or feature request.
3. Let us know in the issue that you plan on creating a pull request for it, by selecting the option when creating an issue. This helps us to keep track of the pull request and make sure there isn't duplicate effort.
4. It's better to wait for feedback from someone on Netapp's Terraform Team development team before writing code. We don't have an SLA for our feedback, but we will do our best to respond in a timely manner (at a minimum, to give you an idea if you're on the right track and that you should proceed, or not).
5. Sign and submit [NetApp's Contributor License Agreement (CCLA)](https://netapp.tap.thinksmart.com/prod/Portal/ShowWorkFlow/AnonymousEmbed/3d2f3aa5-9161-4970-997d-e482b0b033fa). You must sign and submit the Corporate Contributor License Agreement (CCLA) in order to contribute.

   * For Project name, select "Terraform Provider for StorageGRID"
   * For Project Website, enter "https://github.com/NetApp/terraform-provider-netapp-storagegrid"
6. If you've made it this far, have written the code that solves your issue, and addressed the review comments, then feel free to create your pull request.

Important: NetApp will NOT look at the PR or any of the code submitted in the PR if the CCLA is not on file with NetApp Legal.

## Requirement for new Resources/Data Sources

If your building a new Resources or Data Source for StorageGRID we have a few requirements that need to be met before we can accept the PR.

* documentation in the /docs directory, generated with `python3 scripts/generate_docs.py`. The directory is generated, do not edit it: the guides are in /templates/guides, and the pages of the resources and data sources come from their schema and from their example.
* example in the /examples directory
* ACCtest in the /internal/provider directory
  * we have a GitHub Self Hosted Action that will run the ACCtest on an internal StorageGRID system, if there is anything that need to be set up for the ACCtest to run please let us know in the PR.
* Pass all existing GitHub actions test

# Adding a Resource or a Data Source

## Naming

A resource or a data source is named after the API it uses, as the modules of the [netapp.storagegrid Ansible collection](https://github.com/ansible-collections/netapp.storagegrid) are. A name starts with:

* `grid_` for the Grid Management API (`grid/...`). It needs a connection profile without `account_id`. `na_sg_grid_account` is `grid_account`.
* `org_` for the Tenant Management API (`org/...`). It needs a connection profile with the `account_id` of a tenant. `na_sg_org_container` is `org_container`.

The type name is `netapp-storagegrid_<name>`, eg `netapp-storagegrid_grid_account`. A data source that lists them is named in the plural, eg `grid_accounts`.
The code of `grid_` goes in `internal/provider/grid`, and the code of `org_` goes in `internal/provider/org`.

The modules of the Ansible collection are a good reference for how the APIs behave, and for the fields that they accept.
The documentation of the APIs is in the Grid Manager and in the Tenant Manager (help menu, API documentation), where they can be tried.

## Create the skeleton

```shell
./scripts/create_resource.bash grid_account GridAccount grid/accounts
./scripts/create_data_source.bash grid_account GridAccount grid/accounts
```

The scripts ask for what is not on the command line. They never change a file that exists, and they create:

| File | What |
| --- | --- |
| `internal/interfaces/<name>.go` | the calls to the API, and the models of the data. Shared by the resource and the data sources |
| `internal/provider/<grid or org>/<name>_resource.go` | the resource |
| `internal/provider/<grid or org>/<name>_data_source.go` | the data source for one object, and `<names>_data_source.go` for the list |
| `internal/provider/<grid or org>/*_test.go` | the acceptance tests |
| `examples/resources/netapp-storagegrid_<name>` and `examples/data-sources/...` | the examples, that are also run by the documentation generator |

Then:

1. Complete the generated files, and remove the `TODO` comments. The check of the pull requests fails while there is one.
1. Register the resource and the data sources in `internal/provider/provider.go`. The script tells you what to add.
1. Add the prefix of the name to `CATEGORIES` in `scripts/generate_docs.py` if it is not there already.
1. Run `python3 scripts/generate_docs.py` to generate the documentation.
1. Add the new resource or data source to `CHANGELOG.md`.

## Tests

* `./scripts/unittest.sh` runs the unit tests. They use a StorageGRID that runs in the test, and not a real one. Some of them (the data source `grid_info`) start the `terraform` binary, and are skipped without it. Terraform reaches the provider with a unix socket in the temporary directory, whose path is limited to 104 characters on macOS: the tests use `/tmp` when `TMPDIR` is longer, but `python3 scripts/generate_docs.py` needs a short `TMPDIR` too.
* `./scripts/acctest.sh` runs the acceptance tests of the grid or of the tenant resources, with a report of the coverage. They need a StorageGRID system: see the top of [scripts/acctest.sh](scripts/acctest.sh) for the environment variables. The tests of `grid_` resources use a grid profile, and the tests of `org_` resources use a tenant profile: set `TF_ACC_STORAGEGRID_ACCOUNT_ID` to run them. `make testacc` (the default target of the GNUmakefile, as in the ONTAP provider) runs `go test ./...` with `TF_ACC=1`, for all the packages.
* The checks of the pull requests, except the acceptance tests, are `go build ./...`, `go vet ./...`, [staticcheck](https://staticcheck.dev) (`go run honnef.co/go/tools/cmd/staticcheck@v0.8.2 ./...`), the unit tests, `./scripts/check_scaffold.sh` (the templates of the scripts still build) and `./scripts/check_docs.sh` (the documentation is complete).

If you change the REST client, or the helpers that the templates in `scripts/templates` use, `./scripts/check_scaffold.sh` tells you when the templates need to change too.

# Workflows

The workflows of `.github/workflows` run on the pull requests, and on the branches `main`, `integration/main` and `stable*`.

| Workflow | What it checks |
| --- | --- |
| `build.yml` | `go mod tidy` has nothing to change, the provider builds, and the templates of the scripts still build |
| `unit-test.yml` | the unit tests (`scripts/unittest.sh`) |
| `lint.yml` | gofmt, `go vet` and staticcheck |
| `reportcard.yml` | the [Go Report Card](https://goreportcard.com) score is at least 90 (gofmt, vet, gocyclo, golint, ineffassign, license, staticcheck, errcheck). Keep the functions simple: gocyclo reports more than 15 |
| `check_code.yml` | the changed Go files have no `TODO` comment |
| `check_for_documentation.yml` | every resource and data source has its page and its example, and `scripts/generate_docs.py` has nothing to change |
| `check_for_examples.yml` | the same checks as `scripts/check_docs.sh`, and a comment on the pull request with what is missing |
| `codeql.yml`, `gosec.yml` | the code scanning of GitHub |
| `acc_test.yml` | the acceptance tests, with a self-hosted runner that reaches a StorageGRID system. They run when the repository variable `STORAGEGRID_ACC_TESTS` is `true`, and the secrets `TF_ACC_STORAGEGRID_HOST`, `TF_ACC_STORAGEGRID_USER` and `TF_ACC_STORAGEGRID_PASS` are set (see the workflow for the tenant tests) |
| `blackduck.yml` | the Black Duck scan of the dependencies, with a self-hosted runner. It runs when the repository variable `BLACKDUCK_SCAN` is `true`, and the secrets `BLACKDUCK_URL` and `BLACKDUCK_API_TOKEN` are set. Set `DETECT_PROJECT_VERSION_NAME` in `scripts/bd_scan.bash` to the version that you are going to release: the workflow checks that it is newer than the latest release tag |

# Netapp's Terraform Team's Commitment

While we truly appreciate your efforts on pull requests, we cannot make any commitment for when, or if, your PR will be included in this project. Here are a few reasons why:

* There are many factors involved in integrating new code into this project, including things like: support for a wide variety of NetApp backends, proper adherence to our existing and/or upcoming architecture, sufficient functional and/or scenario tests across all backends, etc. In other words, while your bug fix or feature may be perfect as a standalone patch, we must ensure that your code works and can be accommodated for in all of our use cases, configurations, backends, installer, support matrix, etc.
* The Terraform Team must plan our resources to integrate your code into our code base and CI platform, and depending on the complexity of your PR, we may or may not have the resources available to make it happen in a timely fashion. As much as we would like to, and as much as we like to be "agile", we still have a roadmap of features that comes from Product Management. We will do our best, but it doesn't always happen quickly.
* Sometimes the PR just doesn't fit into our future plans. As stated above, we have a roadmap of things, and you don't necessarily know what's coming down the pike. It's possible that a PR you submit doesn't align with our upcoming plans, thus we won't be able to use it. It's not personal.

Thank you for considering to contribute to the Terraform Provider for StorageGRID!
