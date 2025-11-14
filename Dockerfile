FROM gcr.io/distroless/static-debian13:nonroot
WORKDIR /
COPY bin/server /server
ENTRYPOINT ["/server"]
