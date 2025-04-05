FROM ubuntu:noble

COPY betterbuttons /app/betterbuttons

ENTRYPOINT ["/app/betterbuttons"]
