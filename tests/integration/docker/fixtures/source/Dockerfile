ARG BASE_IMAGE=ubuntu:24.04
FROM ${BASE_IMAGE}
ARG SSH_PUBLIC_KEY

COPY artisan /tmp/hostshift-artisan

RUN apt-get update \
  && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    openssh-server \
    ca-certificates \
    curl \
    findutils \
    iproute2 \
    mariadb-client \
    mariadb-server \
    postgresql \
    postgresql-client \
    procps \
    python3 \
    rsync \
    tar \
    util-linux \
  && rm -rf /var/lib/apt/lists/* \
  && mkdir -p /run/sshd /root/.ssh \
  && chmod 700 /root/.ssh \
  && printf '%s\n' "$SSH_PUBLIC_KEY" > /root/.ssh/authorized_keys \
  && chmod 600 /root/.ssh/authorized_keys \
  && mkdir -p /etc/ssh/sshd_config.d \
  && printf 'Port 2222\nPermitRootLogin yes\nPasswordAuthentication no\nPubkeyAuthentication yes\nUsePAM no\nAuthorizedKeysFile .ssh/authorized_keys\n' > /etc/ssh/sshd_config.d/99-hostshift.conf \
  && mkdir -p /fixture/hostshift /srv/app/config /srv/app/public /srv/app/fixtures/mysql /srv/app/fixtures/postgresql /etc/nginx/sites-available \
  && printf 'source-marker\n' > /fixture/hostshift/source-immutability.txt \
  && printf 'ok\n' > /srv/app/health.txt \
  && printf 'APP_NAME=HostShiftFixture\nAPP_ENV=production\nDB_CONNECTION=mysql\nDB_DATABASE=fixturedb\nDB_USERNAME=root\nDB_SOCKET=/run/mysqld/mysqld.sock\n' > /srv/app/.env \
  && cp /srv/app/.env /srv/app/.env.example \
  && cp /tmp/hostshift-artisan /srv/app/artisan \
  && printf 'services:\n  app:\n    image: nginx:alpine\n    container_name: fixture-app\n    ports:\n      - "8080:80"\n    volumes:\n      - ./public:/usr/share/nginx/html:ro\n' > /srv/app/docker-compose.yml \
  && printf '{\n  "service": "fixture-standalone",\n  "image": "fixture/standalone:latest"\n}\n' > /srv/app/config/standalone.json \
  && printf '<!doctype html><html><body>hostshift fixture</body></html>\n' > /srv/app/public/index.html \
  && printf 'ok\n' > /srv/app/public/health \
  && printf 'CREATE DATABASE fixturedb;\nUSE fixturedb;\nCREATE TABLE pages (id INT PRIMARY KEY, slug VARCHAR(64));\nINSERT INTO pages VALUES (1, "home"), (2, "health");\n' > /srv/app/fixtures/mysql/fixturedb.sql \
  && printf 'CREATE TABLE metrics (id integer primary key, name text);\nINSERT INTO metrics VALUES (1, '\''uptime'\''), (2, '\''traffic'\'');\n' > /srv/app/fixtures/postgresql/fixturedb.sql \
  && printf 'server { listen 80; server_name example.test; }\n' > /etc/nginx/sites-available/example.conf \
  && printf 'ClientAliveInterval 120\nClientAliveCountMax 720\n' > /fixture/hostshift/sshd_config \
  && printf 'bind-address = 172.17.0.1\nmysqlx-bind-address = 127.0.0.1\n' > /fixture/hostshift/mysqld.cnf \
  && sha256sum \
    /fixture/hostshift/source-immutability.txt \
    /fixture/hostshift/sshd_config \
    /fixture/hostshift/mysqld.cnf \
    /srv/app/.env \
    /srv/app/.env.example \
    /srv/app/artisan \
    /srv/app/docker-compose.yml \
    /srv/app/config/standalone.json \
    /srv/app/health.txt \
    /srv/app/public/index.html \
    /srv/app/public/health \
    /srv/app/fixtures/mysql/fixturedb.sql \
    /srv/app/fixtures/postgresql/fixturedb.sql \
    /etc/nginx/sites-available/example.conf \
    > /fixture/hostshift/source.sha256

COPY entrypoint.sh /usr/local/bin/hostshift-source-entrypoint.sh
RUN chmod 0644 /srv/app/artisan \
  && chmod +x /usr/local/bin/hostshift-source-entrypoint.sh

CMD ["/usr/local/bin/hostshift-source-entrypoint.sh"]
