FROM gcr.io/distroless/static-debian12

COPY betterbuttons /

ENTRYPOINT ["/betterbuttons"]
