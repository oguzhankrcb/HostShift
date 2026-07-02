import test from "node:test";
import assert from "node:assert/strict";
import {
  assertAbsoluteTransferPath,
  assertReadOnlySourceCommand,
  buildDatabaseReadCommand,
  buildContainerMysqlDumpCommand,
  buildDockerImageSaveCommand,
  buildSourceTarCommand,
  sourceFactCommand
} from "../src/safety.js";

test("source fact commands only accept allowlisted names", () => {
  assert.match(sourceFactCommand("osRelease"), /os-release/);
  assert.match(sourceFactCommand("ufwStatus"), /ufw/);
  assert.match(sourceFactCommand("sshdEffectiveConfig"), /sshd/);
  assert.match(sourceFactCommand("mysqlServerConfig"), /mysqld\.cnf/);
  assert.match(sourceFactCommand("mysqlDatabases"), /SHOW DATABASES/);
  assert.match(sourceFactCommand("nginxConfigDump"), /nginx/);
  assert.throws(() => sourceFactCommand("arbitrary"), /not allowlisted/);
});

test("source command guard rejects mutations", () => {
  assert.throws(() => assertReadOnlySourceCommand("sudo systemctl stop nginx"), /forbidden/);
  assert.throws(() => assertReadOnlySourceCommand("cat /etc/passwd > /tmp/passwd"), /forbidden/);
});

test("transfer paths reject machine identity and broad roots", () => {
  assert.throws(() => assertAbsoluteTransferPath("/"), /too broad/);
  assert.throws(() => assertAbsoluteTransferPath("/etc/machine-id"), /machine-specific/);
  assert.throws(() => assertAbsoluteTransferPath("/var/lib/docker/volumes/a"), /machine-specific/);
  assert.equal(assertAbsoluteTransferPath("/srv/example"), "/srv/example");
});

test("tar stream reads source paths through stdout", () => {
  const command = buildSourceTarCommand(["/srv/example", "/etc/nginx/sites-enabled"]);
  assert.match(command, /^tar --create --file=-/);
  assert.doesNotMatch(command, /sudo|>/);
});

test("database commands stream without source files", () => {
  assert.match(buildDatabaseReadCommand({ engine: "mysql", name: "app" }), /single-transaction/);
  assert.match(buildDatabaseReadCommand({ engine: "postgresql", name: "app" }), /format=custom/);
  assert.throws(() => buildDatabaseReadCommand({ engine: "redis", name: "0" }), /snapshot or replica/);
});

test("container database and image streams are typed read-only operations", () => {
  const dump = buildContainerMysqlDumpCommand({
    sourceContainer: "app-db",
    userEnv: "MYSQL_USER",
    passwordEnv: "MYSQL_PASSWORD",
    databaseEnv: "MYSQL_DATABASE"
  });
  assert.match(dump, /^docker exec 'app-db' sh -c/);
  assert.match(dump, /mysqldump/);
  assert.doesNotMatch(dump, />/);
  assert.equal(buildDockerImageSaveCommand("portfolio:latest"), "docker image save 'portfolio:latest'");
});
