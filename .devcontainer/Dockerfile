FROM mcr.microsoft.com/devcontainers/go:1-1.24-bookworm

# [Optional] Uncomment this section to install additional OS packages.
# RUN apt-get update && export DEBIAN_FRONTEND=noninteractive \
#     && apt-get -y install --no-install-recommends <your-package-list-here>

# [Optional] Uncomment the next lines to use go get to install anything else you need
# USER vscode
# RUN go get -x <your-dependency-or-tool>
# USER root

# [Optional] Uncomment this line to install global node packages.
# RUN su vscode -c "source /usr/local/share/nvm/nvm.sh && npm install -g <your-package-here>" 2>&1

# Install required packages
RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates curl iputils-ping gnupg apt-transport-https

# Install Java 17 for Firestore emulator and configure environment
RUN apt-get update && \
    apt-get install -y openjdk-17-jdk && \
    echo "JAVA_HOME=/usr/lib/jvm/java-17-openjdk-amd64" >> /etc/environment && \
    echo "PATH=$PATH:$JAVA_HOME/bin" >> /etc/environment

# Verify Java installation and environment
RUN . /etc/environment && \
    java -version && \
    echo $JAVA_HOME

# Install Google Cloud CLI
RUN echo "deb [signed-by=/usr/share/keyrings/cloud.google.gpg] https://packages.cloud.google.com/apt cloud-sdk main" | tee -a /etc/apt/sources.list.d/google-cloud-sdk.list && \
    curl https://packages.cloud.google.com/apt/doc/apt-key.gpg | apt-key --keyring /usr/share/keyrings/cloud.google.gpg add - && \
    apt-get update && \
    apt-get install -y google-cloud-cli google-cloud-cli-firestore-emulator

# Install Firebase Tools via npm
RUN curl -fsSL https://deb.nodesource.com/setup_20.x | bash - && \
    apt-get install -y nodejs && \
    npm install -g firebase-tools

# Install taskfile
RUN sh -c "$(curl --location https://taskfile.dev/install.sh)" -- -d -b /usr/local/bin

# Set up environment for vscode user
COPY --chown=vscode:vscode <<-"EOF" /home/vscode/.bashrc.d/java-env
export JAVA_HOME=/usr/lib/jvm/java-17-openjdk-amd64
export PATH=$PATH:$JAVA_HOME/bin
EOF

# Ensure the .bashrc.d directory exists and has correct permissions
RUN mkdir -p /home/vscode/.bashrc.d && \
    chown -R vscode:vscode /home/vscode/.bashrc.d && \
    chmod +x /home/vscode/.bashrc.d/java-env

