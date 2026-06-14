FROM alpine:3.19

RUN apk add --no-cache ca-certificates

COPY flowtest /usr/local/bin/flowtest
COPY flowtest.yaml /etc/flowtest/flowtest.yaml
COPY flows /usr/share/flowtest/flows

WORKDIR /workspace

ENTRYPOINT ["flowtest"]
CMD ["--help"]
