FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /
COPY bin/server /server
ENTRYPOINT ["/server"]
