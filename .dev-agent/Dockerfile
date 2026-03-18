FROM node:24

ARG CLAUDE_CODE_VERSION=stable

# Install development tools, iptables for firewall
RUN apt-get update && apt-get install -y --no-install-recommends \
    git \
    curl \
    sudo \
    iptables \
    iproute2 \
    jq \
    gh \
    vim \
    less \
    procps \
    ca-certificates \
    unzip \
    openssh-client \
    zip \
    python3 \
    make \
    ripgrep \
    tmux \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

# Install Go
ARG GO_VERSION=1.23.8
RUN ARCH=$(uname -m) && \
    if [ "$ARCH" = "x86_64" ]; then GO_ARCH="amd64"; elif [ "$ARCH" = "aarch64" ]; then GO_ARCH="arm64"; else echo "Unsupported arch: $ARCH" && exit 1; fi && \
    curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${GO_ARCH}.tar.gz" -o /tmp/go.tar.gz && \
    tar -C /usr/local -xzf /tmp/go.tar.gz && \
    rm /tmp/go.tar.gz

# Install golangci-lint
RUN curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b /usr/local/bin

# Install AWS CLI v2 (for Bedrock authentication)
RUN ARCH=$(uname -m) && \
    curl -fsSL "https://awscli.amazonaws.com/awscli-exe-linux-${ARCH}.zip" -o /tmp/awscliv2.zip && \
    unzip -q /tmp/awscliv2.zip -d /tmp && \
    /tmp/aws/install && \
    rm -rf /tmp/aws /tmp/awscliv2.zip

# Enable corepack so pnpm/yarn are available
RUN corepack enable

# Create workspace and config directories
RUN mkdir -p /workspace /home/node/.claude /home/node/.local/bin && \
    chown -R node:node /workspace /home/node/.claude /home/node/.local

WORKDIR /workspace

# Bake skills into the image (from dev-agent build context)
COPY --chown=node:node dot-claude/skills/ /home/node/.claude/skills/

# Install Claude Code and Go tools (as node user)
USER node
ENV PATH="/usr/local/go/bin:/home/node/go/bin:/home/node/.local/bin:$PATH"
ENV GOPATH="/home/node/go"
RUN curl -fsSL https://claude.ai/install.sh | bash -s -- ${CLAUDE_CODE_VERSION}

# Pre-create GOPATH directories
RUN mkdir -p /home/node/go/bin /home/node/go/pkg /home/node/go/src

# Trust mounted volumes (ownership differs between host and container)
RUN git config --global --add safe.directory /workspace && \
    git config --global --add safe.directory /main-repo

# Copy firewall and entrypoint scripts (needs root; from dev-agent build context)
USER root
COPY init-firewall.sh /usr/local/bin/
COPY entrypoint.sh /usr/local/bin/
RUN chmod 0555 /usr/local/bin/init-firewall.sh && \
    chown root:root /usr/local/bin/init-firewall.sh && \
    chmod +x /usr/local/bin/entrypoint.sh && \
    echo "node ALL=(root) NOPASSWD: /usr/local/bin/init-firewall.sh" > /etc/sudoers.d/node-firewall && \
    chmod 0440 /etc/sudoers.d/node-firewall

USER node

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD ["sleep", "infinity"]
