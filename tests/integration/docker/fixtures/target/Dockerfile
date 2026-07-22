ARG BASE_IMAGE=ubuntu:24.04
FROM ${BASE_IMAGE}

RUN find /etc/apt -type f \( -name '*.list' -o -name '*.sources' \) \
    -exec sed -i \
      -e 's|http://archive.ubuntu.com/ubuntu|https://us.archive.ubuntu.com/ubuntu|g' \
      -e 's|http://security.ubuntu.com/ubuntu|https://us.archive.ubuntu.com/ubuntu|g' {} + \
  && apt-get -o Acquire::Retries=5 update \
  && DEBIAN_FRONTEND=noninteractive apt-get -o Acquire::Retries=5 install -y --no-install-recommends \
    openssh-server \
    ca-certificates \
    curl \
    findutils \
    iproute2 \
    mariadb-client \
    mariadb-server \
    php-cli \
    php-mysql \
    postgresql \
    postgresql-client \
    procps \
    python3 \
    rsync \
    tar \
    util-linux \
  && rm -rf /var/lib/apt/lists/*

ARG SSH_PUBLIC_KEY
RUN mkdir -p /run/sshd /root/.ssh /fixture/hostshift /etc/ssh/sshd_config.d \
  && chmod 700 /root/.ssh \
  && printf '%s\n' "$SSH_PUBLIC_KEY" > /root/.ssh/authorized_keys \
  && chmod 600 /root/.ssh/authorized_keys \
  && printf 'Port 2222\nPermitRootLogin yes\nPasswordAuthentication no\nPubkeyAuthentication yes\nUsePAM no\nAuthorizedKeysFile .ssh/authorized_keys\n' > /etc/ssh/sshd_config.d/99-hostshift.conf

COPY docker-shim.sh /usr/local/bin/docker
COPY entrypoint.sh /usr/local/bin/hostshift-target-entrypoint.sh
RUN chmod +x /usr/local/bin/docker /usr/local/bin/hostshift-target-entrypoint.sh

CMD ["/usr/local/bin/hostshift-target-entrypoint.sh"]
