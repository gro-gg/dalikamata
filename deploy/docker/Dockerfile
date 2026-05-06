FROM golang:1.26 AS build

WORKDIR /go/src/app
COPY . .

RUN go mod download
RUN CGO_ENABLED=0 go build -o /go/bin/dalikamata

FROM gcr.io/distroless/static-debian13

COPY --from=build /go/bin/dalikamata /

EXPOSE 4222

ENTRYPOINT ["/dalikamata"]
CMD ["-h"]
