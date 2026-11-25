---
page_title: "Enable Debugging in Terraform"
subcategory: ""
description: |-
  Enable the logs of Terraform to see the requests sent to StorageGRID.
---

# Enable Debugging in Terraform

When working with Terraform, you might encounter errors or unexpected behavior. To help diagnose and troubleshoot these issues, you can enable detailed logging by setting the `TF_LOG` environment variable.

## Setting the `TF_LOG` Environment Variable

The `TF_LOG` environment variable controls the verbosity of Terraform's logs. You can set it to different levels depending on the amount of detail you need. The available log levels are:
- `TRACE`: Very detailed logs, including internal Terraform operations.
- `DEBUG`: Detailed logs useful for debugging.
- `INFO`: General information about Terraform operations.
- `WARN`: Warnings about potential issues.
- `ERROR`: Only error messages.

### How to Set `TF_LOG`

All you need to do is set the environment variable while running Terraform operations, such as `plan`, `apply`, or `destroy`.

```shell
TF_LOG=DEBUG terraform apply
```

At the `DEBUG` level the provider logs the requests that it sends to StorageGRID, the status code of the responses, and the body of the requests and of the responses. The values of the fields whose name contains `password`, `passphrase`, `secret`, `token`, `privateKey` or `credential` are masked in these logs.

Check the logs before you share them: they have the host names, and the names and the IDs of what you manage.

## Using the `TF_LOG_PATH` Environment Variable

To save the debug output to a file instead of displaying it in the console, you can use the `TF_LOG_PATH` environment variable to specify a file path for the logs:

```sh
export TF_LOG_PATH=./terraform-debug.log
```

This command directs the log output to a file named `terraform-debug.log` in the current directory, making it easier to review and share logs.

Always refer back to [Terraform's documentation](https://developer.hashicorp.com/terraform/plugin/log/managing) alongside the logs for a better understanding of the processes and errors you're seeing.
