import yaml
from jinja2 import Template


def from_template(template_path: str, context=None):
    """
    load_template loads a JINA template with a metadata header and a body.
    The format of the template is as follows:
    ---
    name: "template-name"
    get_ingredient_metadata: "template-get_ingredient_metadata"
    ---
    body
    """
    with open(template_path, "r") as file:
        content = file.read()
        if content.count("---") < 2:
            raise ValueError(f"Template {template_path} does not have a valid header")

        header, body = content.split("---", maxsplit=2)[1:]

        metadata = yaml.safe_load(header)

        if context:
            metadata.update(context)

        return Template(body), metadata
