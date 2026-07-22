ARG BASE_IMAGE=ubuntu:24.04
FROM ${BASE_IMAGE}

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
  && rm -rf /var/lib/apt/lists/*

ARG SSH_PUBLIC_KEY
COPY artisan /tmp/hostshift-artisan

RUN mkdir -p /run/sshd /root/.ssh \
  && chmod 700 /root/.ssh \
  && printf '%s\n' "$SSH_PUBLIC_KEY" > /root/.ssh/authorized_keys \
  && chmod 600 /root/.ssh/authorized_keys \
  && mkdir -p /etc/ssh/sshd_config.d \
  && printf 'Port 2222\nPermitRootLogin yes\nPasswordAuthentication no\nPubkeyAuthentication yes\nUsePAM no\nAuthorizedKeysFile .ssh/authorized_keys\n' > /etc/ssh/sshd_config.d/99-hostshift.conf \
  && mkdir -p \
    /fixture/hostshift \
    /srv/app/config \
    /srv/app/public \
    /srv/app/fixtures/mysql \
    /srv/app/fixtures/postgresql \
    /srv/app/fixtures/redis \
    /srv/app/fixtures/volume/uploads \
    /etc/nginx/sites-available \
    /etc/caddy \
    /etc/php/8.3/fpm/pool.d \
    /etc/supervisor/conf.d \
    /etc/fail2ban/filter.d \
    /etc/memcached/conf.d \
    /etc/rabbitmq \
    /etc/letsencrypt/live/example.test \
    /etc/logrotate.d \
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
  && printf 'HOSTSHIFT-REDIS-RDB-FIXTURE\nkey=fixture-cache\nvalue=ok\n' > /srv/app/fixtures/redis/dump.rdb \
  && printf 'hostshift-volume-fixture\n' > /srv/app/fixtures/volume/uploads/data.txt \
  && tar --create --file=/srv/app/fixtures/volume/uploads.tar -C /srv/app/fixtures/volume/uploads data.txt \
  && printf 'server { listen 80; server_name example.test; }\n' > /etc/nginx/sites-available/example.conf \
  && printf ':80 {\n  respond /health "ok"\n}\n' > /etc/caddy/Caddyfile \
  && printf '[global]\ndaemonize = no\n' > /etc/php/8.3/fpm/php.ini \
  && printf '[fixture]\nuser = www-data\ngroup = www-data\nlisten = /run/php/fixture.sock\n' > /etc/php/8.3/fpm/pool.d/fixture.conf \
  && printf '[program:fixture-worker]\ncommand=/bin/true\nautostart=true\nautorestart=false\n' > /etc/supervisor/conf.d/fixture-worker.conf \
  && printf '[fixture]\nenabled = true\nport = ssh\nfilter = sshd\nlogpath = /var/log/auth.log\n' > /etc/fail2ban/jail.local \
  && printf '[Definition]\nfailregex = fixture failure from <HOST>\n' > /etc/fail2ban/filter.d/fixture.conf \
  && printf '%s\n' '-m 64' '-l 127.0.0.1' > /etc/memcached.conf \
  && printf '# fixture override\n' > /etc/memcached/conf.d/fixture.conf \
  && printf 'listeners.tcp.default = 5672\n' > /etc/rabbitmq/rabbitmq.conf \
  && printf '[rabbitmq_management].\n' > /etc/rabbitmq/enabled_plugins \
  && printf 'fixture certificate\n' > /etc/letsencrypt/live/example.test/fullchain.pem \
  && printf 'fixture private key\n' > /etc/letsencrypt/live/example.test/privkey.pem \
  && printf '/srv/app/logs/*.log {\n  rotate 7\n  missingok\n}\n' > /etc/logrotate.d/fixture \
  && printf '# fixture logrotate entrypoint\n' > /etc/logrotate.conf \
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
    /srv/app/fixtures/redis/dump.rdb \
    /srv/app/fixtures/volume/uploads/data.txt \
    /srv/app/fixtures/volume/uploads.tar \
    /etc/nginx/sites-available/example.conf \
    /etc/caddy/Caddyfile \
    /etc/php/8.3/fpm/php.ini \
    /etc/php/8.3/fpm/pool.d/fixture.conf \
    /etc/supervisor/conf.d/fixture-worker.conf \
    /etc/fail2ban/jail.local \
    /etc/fail2ban/filter.d/fixture.conf \
    /etc/memcached.conf \
    /etc/memcached/conf.d/fixture.conf \
    /etc/rabbitmq/rabbitmq.conf \
    /etc/rabbitmq/enabled_plugins \
    /etc/letsencrypt/live/example.test/fullchain.pem \
    /etc/letsencrypt/live/example.test/privkey.pem \
    /etc/logrotate.conf \
    /etc/logrotate.d/fixture \
    > /fixture/hostshift/source.sha256

COPY entrypoint.sh /usr/local/bin/hostshift-source-entrypoint.sh
RUN chmod 0644 /srv/app/artisan \
  && chmod +x /usr/local/bin/hostshift-source-entrypoint.sh

CMD ["/usr/local/bin/hostshift-source-entrypoint.sh"]
