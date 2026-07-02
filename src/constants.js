export const VERSION = "0.3.0";

export const SOURCE_FACTS = Object.freeze({
  osRelease: ["cat", "/etc/os-release"],
  architecture: ["uname", "-m"],
  hostname: ["hostname"],
  disk: ["df", "-Pk"],
  memory: ["cat", "/proc/meminfo"],
  packages: ["dpkg-query", "-W", "-f=${binary:Package}\\t${Version}\\n"],
  enabledServices: [
    "systemctl",
    "list-unit-files",
    "--state=enabled",
    "--type=service",
    "--no-pager",
    "--no-legend"
  ],
  runningServices: [
    "systemctl",
    "list-units",
    "--state=running",
    "--type=service",
    "--no-pager",
    "--no-legend"
  ],
  mounts: ["findmnt", "--json", "--real"],
  listeners: ["ss", "-lntupH"],
  ufwStatus: ["ufw", "status", "verbose"],
  sshdEffectiveConfig: ["sshd", "-T"],
  sshdConfig: ["cat", "/etc/ssh/sshd_config"],
  mysqlServerConfig: ["cat", "/etc/mysql/mysql.conf.d/mysqld.cnf"],
  mysqlDatabases: ["mysql", "--batch", "--skip-column-names", "--execute=SHOW DATABASES"],
  nginxConfigDump: ["nginx", "-T"],
  letsEncryptFiles: ["find", "/etc/letsencrypt", "-maxdepth", "3", "-type", "f", "-print"],
  users: ["getent", "passwd"],
  groups: ["getent", "group"],
  cron: ["find", "/etc/cron.d", "/etc/cron.daily", "/etc/cron.hourly", "/etc/cron.monthly", "/etc/cron.weekly", "-maxdepth", "1", "-type", "f", "-print"],
  dockerVersion: ["docker", "version", "--format", "{{json .Server.Version}}"],
  dockerComposeProjects: ["docker", "compose", "ls", "--format", "json"],
  dockerContainers: ["docker", "ps", "--format", "{{json .}}"],
  dockerNetworks: ["docker", "network", "ls", "--format", "{{json .}}"]
});

export const SOURCE_FORBIDDEN_TOKENS = Object.freeze([
  "sudo", "su", "doas", "systemctl start", "systemctl stop",
  "systemctl restart", "systemctl reload", "service ", "kill", "pkill",
  "apt", "apt-get", "dpkg -i", "snap install", "docker stop",
  "docker restart", "docker rm", "docker exec", "tee", "touch", "mkdir",
  "rm ", "mv ", "cp ", "chmod", "chown", "truncate", "sed -i",
  "mysql -e", "psql -c", "redis-cli set", ">", ">>"
]);

export const MACHINE_SPECIFIC_EXCLUDES = Object.freeze([
  "/etc/machine-id",
  "/var/lib/dbus/machine-id",
  "/etc/ssh/ssh_host_*",
  "/etc/netplan",
  "/var/lib/cloud",
  "/etc/fstab",
  "/boot",
  "/proc",
  "/sys",
  "/dev",
  "/run",
  "/tmp",
  "/var/tmp",
  "/var/lib/docker"
]);

export const PROFILE_VERSION = 1;

export const DEFAULT_TARGET_POLICY = Object.freeze({
  allowedUbuntuVersions: ["24.04", "25.10", "26.04"],
  preferredUbuntuVersions: ["24.04", "26.04"],
  requiredArchitecture: "x86_64"
});

export const DEFAULT_FIREWALL_RULES = Object.freeze([
  { from: "0.0.0.0/0", port: 22, proto: "tcp", comment: "SSH access" },
  { from: "::/0", port: 22, proto: "tcp", comment: "SSH access IPv6" },
  { from: "0.0.0.0/0", port: 80, proto: "tcp", comment: "HTTP access" },
  { from: "::/0", port: 80, proto: "tcp", comment: "HTTP access IPv6" },
  { from: "0.0.0.0/0", port: 443, proto: "tcp", comment: "HTTPS access" },
  { from: "::/0", port: 443, proto: "tcp", comment: "HTTPS access IPv6" },
  { from: "172.21.0.0/16", port: 3306, proto: "tcp", comment: "Docker bridge MySQL access" },
  { from: "172.20.0.0/16", port: 3306, proto: "tcp", comment: "Docker bridge MySQL access" },
  { from: "172.19.0.0/16", port: 3306, proto: "tcp", comment: "Docker bridge MySQL access" },
  { from: "172.18.0.0/16", port: 3306, proto: "tcp", comment: "Docker bridge MySQL access" },
  { from: "127.0.0.1", port: 3306, proto: "tcp", comment: "Local MySQL access" }
]);

export const DEFAULT_SSHD_SETTINGS = Object.freeze({
  ClientAliveInterval: 120,
  ClientAliveCountMax: 720
});

export const DEFAULT_MYSQL_SETTINGS = Object.freeze({
  bindAddress: "172.17.0.1",
  mysqlxBindAddress: "127.0.0.1"
});
