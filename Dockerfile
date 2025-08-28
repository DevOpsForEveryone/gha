ARG BASEIMAGE

FROM ${BASEIMAGE} as base

# Run everything as root

# Install additional tools
RUN apt-get update && apt-get install -y \
    curl \
    wget \
    git \
    unzip \
    zip \
    tar \
    gzip \
    ca-certificates \
    gnupg \
    lsb-release \
    software-properties-common \
    apt-transport-https \
    build-essential \
    sudo \
    jq \
    net-tools \
    iputils-ping \
    dnsutils \
    telnet \
    openssh-client \
    rsync \
    vim \
    nano \
    htop \
    tree \
    && rm -rf /var/lib/apt/lists/*

# Install Docker CLI (if not present)
RUN if ! command -v docker &> /dev/null; then \
        curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /usr/share/keyrings/docker-archive-keyring.gpg && \
        echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" | tee /etc/apt/sources.list.d/docker.list > /dev/null && \
        apt-get update && \
        apt-get install -y docker-ce-cli && \
        rm -rf /var/lib/apt/lists/*; \
    fi

# Install Node.js 18 LTS (if not present)
RUN if ! command -v node &> /dev/null; then \
        curl -fsSL https://deb.nodesource.com/setup_18.x | bash - && \
        apt-get install -y nodejs && \
        npm install -g yarn pnpm; \
    fi

# Install Python 3.11 (if not present)
RUN if ! python3.11 --version &> /dev/null; then \
        add-apt-repository ppa:deadsnakes/ppa && \
        apt-get update && \
        apt-get install -y python3.11 python3.11-venv python3.11-dev python3-pip && \
        ln -sf /usr/bin/python3.11 /usr/bin/python3 && \
        ln -sf /usr/bin/python3.11 /usr/bin/python && \
        python3 -m pip install --upgrade pip setuptools wheel && \
        rm -rf /var/lib/apt/lists/*; \
    fi

# Install Go 1.21 (if not present)
RUN if ! command -v go &> /dev/null; then \
        wget https://go.dev/dl/go1.21.5.linux-amd64.tar.gz && \
        tar -C /usr/local -xzf go1.21.5.linux-amd64.tar.gz && \
        rm go1.21.5.linux-amd64.tar.gz; \
    fi
ENV PATH="/usr/local/go/bin:${PATH}"

# Install GitHub CLI (if not present)
RUN if ! command -v gh &> /dev/null; then \
        curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg && \
        chmod go+r /usr/share/keyrings/githubcli-archive-keyring.gpg && \
        echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | tee /etc/apt/sources.list.d/github-cli.list > /dev/null && \
        apt-get update && \
        apt-get install -y gh && \
        rm -rf /var/lib/apt/lists/*; \
    fi

# Install AWS CLI v2 (if not already present)
RUN if ! command -v aws &> /dev/null; then \
        curl "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" -o "awscliv2.zip" && \
        unzip awscliv2.zip && \
        ./aws/install && \
        rm -rf aws awscliv2.zip; \
    fi

# Install kubectl (if not already present)
RUN if ! command -v kubectl &> /dev/null; then \
        curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl" && \
        install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl && \
        rm kubectl; \
    fi

# Install Helm (if not already present)
RUN if ! command -v helm &> /dev/null; then \
        curl https://baltocdn.com/helm/signing.asc | gpg --dearmor | tee /usr/share/keyrings/helm.gpg > /dev/null && \
        echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/helm.gpg] https://baltocdn.com/helm/stable/debian/ all main" | tee /etc/apt/sources.list.d/helm-stable-debian.list && \
        apt-get update && \
        apt-get install -y helm && \
        rm -rf /var/lib/apt/lists/*; \
    fi

# Install Terraform (if not already present)
RUN if ! command -v terraform &> /dev/null; then \
        wget -O- https://apt.releases.hashicorp.com/gpg | gpg --dearmor | tee /usr/share/keyrings/hashicorp-archive-keyring.gpg && \
        echo "deb [signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg] https://apt.releases.hashicorp.com $(lsb_release -cs) main" | tee /etc/apt/sources.list.d/hashicorp.list && \
        apt-get update && \
        apt-get install -y terraform && \
        rm -rf /var/lib/apt/lists/*; \
    fi

# Install additional Python packages
RUN python3 -m pip install \
    requests \
    boto3 \
    pytest \
    black \
    flake8 \
    mypy \
    poetry \
    pipenv

# Keep the original entrypoint from base image
