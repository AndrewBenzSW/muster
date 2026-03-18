FROM ubuntu/squid:latest

# Copy squid configuration
COPY squid.conf /etc/squid/squid.conf
COPY allowed-domains.txt /etc/squid/allowed-domains.txt

# Ensure permissions
RUN chown proxy:proxy /etc/squid/allowed-domains.txt && \
    chmod 644 /etc/squid/allowed-domains.txt

# Squid runs on port 3128 by default
EXPOSE 3128

# Start squid in foreground
CMD ["squid", "-N"]
