FROM scratch

# Microbadger labels
ARG VCS_REF

LABEL org.label-schema.vcs-ref=$VCS_REF \
      org.label-schema.name="confsyncer" \
      org.label-schema.vcs-url="e.g. https://github.com/elafarge/confsyncer"

ADD build/confsyncer /confsyncer
ENTRYPOINT ["/confsyncer"]
