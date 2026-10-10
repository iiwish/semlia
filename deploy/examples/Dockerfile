FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
WORKDIR /app
COPY build/container/semlia /app/semlia
COPY build/container/migrations /app/migrations
COPY --chown=nonroot:nonroot build/container/data/ /var/lib/semlia/
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/app/semlia"]
CMD ["server"]
