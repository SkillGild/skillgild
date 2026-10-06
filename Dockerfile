# The SkillGild MCP server (`skillgild mcp`) as a container, for MCP directories and CI.
# Pass a key from https://skillgild.dev/account as SKILLGILD_API_KEY; the container has
# no OS credential store, so `skillgild login` is not used here.
#
#   docker build -t skillgild .
#   docker run -i --rm -e SKILLGILD_API_KEY skillgild
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY cli/ .
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/skillgild ./cmd/skillgild

FROM alpine:3
RUN apk add --no-cache ca-certificates
COPY --from=build /out/skillgild /usr/local/bin/skillgild
ENTRYPOINT ["skillgild", "mcp"]
