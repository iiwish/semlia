FROM gcr.io/distroless/base-debian12:nonroot@sha256:7f0c72cd138b442ae0deeb69c08b1acf5525439ba251a49ad93c320a061567e5
WORKDIR /app
COPY build/container/semlia /app/semlia
COPY build/container/migrations /app/migrations
COPY --chown=nonroot:nonroot build/container/data/ /var/lib/semlia/
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/app/semlia"]
CMD ["server"]
