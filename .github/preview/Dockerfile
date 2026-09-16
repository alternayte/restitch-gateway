# The release image is distroless and carries no configuration, so a preview
# needs one baked in. It has no shell either, which is why this is a layer
# rather than a setup command in the template.
ARG BASE
FROM ${BASE}
COPY restitch.yaml /restitch.yaml
