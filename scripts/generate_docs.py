#!/usr/bin/env python3
"""Generates the documentation in docs/ with tfplugindocs, and sets the subcategory of the pages.

Run it with "python3 scripts/generate_docs.py", from anywhere in the tree.  It needs the terraform binary, or network access to download it.

docs/ is generated: tfplugindocs removes docs/index.md, docs/resources, docs/data-sources and docs/guides, and renders
them again.  Do not edit it.  Write the guides in templates/guides, and the index page in templates/index.md.tmpl.
The script stops before it generates anything if a guide in docs/guides is not the one of templates/guides.

The subcategory groups the pages in the navigation of the Terraform Registry.  tfplugindocs leaves it empty, and this
script fills it from CATEGORIES.  It fails if a page of a resource or of a data source has no subcategory.
"""
import os
import subprocess
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
PROVIDER_NAME = "netapp-storagegrid"
DOC_DIRECTORIES = ["docs/resources", "docs/data-sources"]

# The subcategory of a page is the one of the longest prefix of the page name that is listed here, so that the
# prefix grid_account also covers grid_accounts.  The pages are named like the resources and the data sources,
# without the provider name: grid_account.md is netapp-storagegrid_grid_account.
#
# The names are those of the modules of the netapp.storagegrid Ansible collection (na_sg_grid_account is
# grid_account), and the groups are the areas of the Grid Manager and of the Tenant Manager.
# Add the prefix of a new resource to one of these groups, or to a new group.
CATEGORIES = {
    "access-control": [
        "grid_group",
        "grid_identity_federation",
        "grid_sso",
        "grid_user",
    ],
    "alerts": [
        "grid_alert_receiver",
    ],
    "certificates": [
        "grid_certificate",
        "grid_client_certificate",
        "grid_self_signed_certificate",
    ],
    "ilm": [
        "grid_ec_profile",
        "grid_ilm",
        "grid_regions",
    ],
    "info": [
        "grid_info",
        "grid_product_version",
        "org_info",
    ],
    "load-balancing": [
        "grid_gateway",
        "grid_ha_group",
        "grid_traffic_class",
    ],
    "monitoring": [
        "grid_audit",
        "grid_autosupport",
        "grid_snmp",
    ],
    "networking": [
        "grid_dns",
        "grid_domain_name",
        "grid_firewall",
        "grid_ntp",
        "grid_proxy",
        "grid_untrusted_client_network",
        "grid_vlan",
    ],
    "tenant-access-control": [
        "org_group",
        "org_identity_federation",
        "org_user",
    ],
    "tenant-buckets": [
        "org_bucket",
        "org_container",
    ],
    "tenant-platform-services": [
        "org_cloud_mirror",
        "org_endpoint",
    ],
    "tenants": [
        "grid_account",
        "grid_tenant",
    ],
}


def main():
    os.chdir(ROOT)
    print("===== Checking the guides =====")
    if check_guides_are_not_lost():
        sys.exit(1)
    print("===== Generating docs =====")
    run_tfplugindocs("generate", "--provider-name", PROVIDER_NAME)
    print("===== Adding Categories =====")
    add_categories()
    print("===== Validate =====")
    run_tfplugindocs("validate")
    print("===== Errors =====")
    if warn_missing_category(DOC_DIRECTORIES):
        sys.exit(1)


def check_guides_are_not_lost():
    """tfplugindocs removes docs/guides, docs/resources, docs/data-sources and docs/index.md, and renders them again.
    The guides are copied from templates/guides: a guide that is only in docs/guides, or that was edited there,
    would be lost.  Prints what would be lost, and returns True if there is something."""
    lost = False
    for directory, _, filenames in os.walk("docs/guides"):
        for filename in sorted(filenames):
            path = os.path.join(directory, filename)
            source = os.path.join("templates", os.path.relpath(path, "docs"))
            if not os.path.isfile(source):
                print("%s would be deleted: move it to %s" % (path, source))
                lost = True
            elif not same_content(path, source):
                print("%s was changed, and the change would be lost: make it in %s" % (path, source))
                lost = True
    return lost


def same_content(first, second):
    with open(first, "rb") as f1, open(second, "rb") as f2:
        return f1.read() == f2.read()


def run_tfplugindocs(*args):
    command = ["go", "run", "github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs"] + list(args)
    environment = os.environ.copy()
    # terraform reaches the provider with a unix socket in the temporary directory, and the path of a socket is limited
    # to 104 characters on macOS: with a longer one, terraform cannot load the schema of the provider
    if len(environment.get("TMPDIR", "")) > 80:
        print("TMPDIR is too long for the unix socket of the provider, using the default one")
        del environment["TMPDIR"]
    try:
        subprocess.run(command, check=True, env=environment)
    except (subprocess.CalledProcessError, FileNotFoundError) as err:
        sys.exit("Error: %s failed: %s" % (" ".join(command), err))


def find_category(page_name):
    """Returns the category of a page name, eg grid_accounts, or None."""
    best_prefix = ""
    best_category = None
    for category, prefixes in CATEGORIES.items():
        for prefix in prefixes:
            if page_name.startswith(prefix) and len(prefix) > len(best_prefix):
                best_prefix = prefix
                best_category = category
    return best_category


def add_categories():
    for directory in DOC_DIRECTORIES:
        if not os.path.isdir(directory):
            continue
        for filename in sorted(os.listdir(directory)):
            if not filename.endswith(".md"):
                continue
            category = find_category(filename[: -len(".md")])
            if category:
                update_md_file(os.path.join(directory, filename), category)


def update_md_file(path, category):
    print("Updating %s" % path)
    with open(path) as f:
        lines = f.readlines()
    for i, line in enumerate(lines):
        if line.startswith("subcategory:"):
            lines[i] = 'subcategory: "%s"\n' % category
            break
    with open(path, "w") as f:
        f.writelines(lines)


def warn_missing_category(directory_paths):
    """Prints the pages that have no subcategory, and returns True if there are some."""
    issue = False
    for directory_path in directory_paths:
        if not os.path.isdir(directory_path):
            continue
        for filename in sorted(os.listdir(directory_path)):
            path = os.path.join(directory_path, filename)
            if not os.path.isfile(path):
                continue
            with open(path) as f:
                if 'subcategory: ""' in f.read():
                    print("%s is missing a category: add its prefix to CATEGORIES in %s" % (path, os.path.relpath(__file__, ROOT)))
                    issue = True
    return issue


if __name__ == "__main__":
    main()
