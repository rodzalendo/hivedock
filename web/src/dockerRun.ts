// dockerRun.ts — convert a `docker run …` command into a compose.yaml snippet.
//
// This is pure string work: it never touches Docker or the network, so it's safe
// to run client-side and easy to unit-test (see dockerRun.test.ts). It covers the
// flags that map cleanly onto a single compose service; anything it doesn't
// understand is reported as a warning rather than silently dropped, so the user
// knows to fill it in by hand on the Compose tab.

export interface ConvertResult {
  yaml: string;
  serviceName: string;
  warnings: string[];
}

// Value flags consume the next token (or an attached =value / -pVALUE). Everything
// else that starts with '-' and isn't a known boolean is reported as a warning.
const VALUE_FLAGS = new Set([
  "--name",
  "-p", "--publish",
  "-e", "--env",
  "--env-file",
  "-v", "--volume",
  "--restart",
  "--network", "--net",
  "-w", "--workdir",
  "-u", "--user",
  "--entrypoint",
  "-l", "--label",
  "--cap-add", "--cap-drop",
  "--device",
  "-h", "--hostname",
  "-m", "--memory",
  "--cpus",
  "--add-host",
  "--dns",
  "--sysctl",
  "--tmpfs",
  "--shm-size",
  "--stop-signal",
  "--health-cmd", "--health-interval", "--health-timeout",
  "--health-retries", "--health-start-period",
]);

const BOOL_FLAGS = new Set([
  "-d", "--detach",
  "--rm",
  "-i", "--interactive",
  "-t", "--tty",
  "--privileged",
  "--init",
  "--read-only",
]);

// Flags we knowingly can't express as a plain compose service — warn and skip so
// the output stays valid rather than half-guessed.
const UNSUPPORTED_VALUE_FLAGS = new Set([
  "--mount",
  "--gpus",
  "--log-driver", "--log-opt",
  "--ulimit",
  "--link",
  "--expose",
  "--platform",
  "--stop-timeout",
]);

// tokenize splits a command line into tokens, honoring single/double quotes and
// backslash line-continuations. Quotes are stripped from the resulting tokens.
export function tokenize(input: string): string[] {
  const tokens: string[] = [];
  let cur = "";
  let quote: '"' | "'" | null = null;
  let has = false; // whether cur holds a (possibly empty) token
  for (let i = 0; i < input.length; i++) {
    const c = input[i];
    if (quote) {
      if (c === quote) quote = null;
      else cur += c;
      continue;
    }
    if (c === '"' || c === "'") {
      quote = c;
      has = true;
      continue;
    }
    if (c === "\\") {
      // A trailing backslash continues the line; otherwise escape the next char.
      const next = input[i + 1];
      if (next === "\n" || next === "\r" || next === undefined) continue;
      cur += next;
      has = true;
      i++;
      continue;
    }
    if (c === " " || c === "\t" || c === "\n" || c === "\r") {
      if (has) {
        tokens.push(cur);
        cur = "";
        has = false;
      }
      continue;
    }
    cur += c;
    has = true;
  }
  if (has) tokens.push(cur);
  return tokens;
}

// splitFlag separates "--flag=value" into ["--flag", "value"]; a short attached
// value like "-eFOO=bar" into ["-e", "FOO=bar"]. Returns [flag, value|null].
function splitFlag(tok: string): [string, string | null] {
  if (tok.startsWith("--")) {
    const eq = tok.indexOf("=");
    if (eq !== -1) return [tok.slice(0, eq), tok.slice(eq + 1)];
    return [tok, null];
  }
  // Short flag: -p, or attached -e VALUE.
  if (tok.length > 2) {
    const short = tok.slice(0, 2);
    if (VALUE_FLAGS.has(short)) return [short, tok.slice(2)];
  }
  return [tok, null];
}

// sanitizeName reduces a container/image name to a valid compose service key.
function sanitizeName(raw: string): string {
  const name = raw
    .replace(/^.*\//, "") // drop registry/org path
    .replace(/:.*$/, "") // drop tag
    .toLowerCase()
    .replace(/[^a-z0-9_-]/g, "-")
    .replace(/^-+|-+$/g, "");
  return name || "app";
}

interface Service {
  image: string;
  restart?: string;
  networkMode?: string;
  workingDir?: string;
  user?: string;
  hostname?: string;
  entrypoint?: string;
  memLimit?: string;
  cpus?: string;
  shmSize?: string;
  stopSignal?: string;
  privileged?: boolean;
  init?: boolean;
  readOnly?: boolean;
  stdinOpen?: boolean;
  tty?: boolean;
  ports: string[];
  environment: string[];
  envFile: string[];
  volumes: string[];
  labels: string[];
  capAdd: string[];
  capDrop: string[];
  devices: string[];
  networks: string[];
  extraHosts: string[];
  dns: string[];
  sysctls: string[];
  tmpfs: string[];
  command: string[];
  health: Record<string, string>;
}

// dockerRunToCompose parses a `docker run` command and returns an equivalent
// single-service compose.yaml, the derived service name, and any warnings.
export function dockerRunToCompose(input: string): ConvertResult {
  const warnings: string[] = [];
  const tokens = tokenize(input);

  // Skip a leading "docker", "sudo", "container", "run".
  let i = 0;
  while (
    i < tokens.length &&
    ["docker", "sudo", "container", "run"].includes(tokens[i])
  ) {
    i++;
  }
  if (i === 0 && tokens.length > 0) {
    warnings.push('Command should start with "docker run".');
  }

  const svc: Service = {
    image: "",
    ports: [], environment: [], envFile: [], volumes: [], labels: [],
    capAdd: [], capDrop: [], devices: [], networks: [], extraHosts: [],
    dns: [], sysctls: [], tmpfs: [], command: [], health: {},
  };
  const namedVolumes = new Set<string>();
  const externalNetworks = new Set<string>();
  let name = "";

  for (; i < tokens.length; i++) {
    const raw = tokens[i];
    if (svc.image) {
      // Everything after the image is the container command.
      svc.command.push(raw);
      continue;
    }
    if (!raw.startsWith("-")) {
      svc.image = raw;
      continue;
    }

    // Split grouped short booleans like -it → -i -t (only when every char is a
    // known boolean flag).
    if (/^-[a-z]{2,}$/i.test(raw) && !raw.startsWith("--")) {
      const chars = raw.slice(1).split("");
      if (chars.every((c) => BOOL_FLAGS.has("-" + c))) {
        for (const c of chars) applyBool("-" + c, svc);
        continue;
      }
    }

    const [flag, attached] = splitFlag(raw);

    if (BOOL_FLAGS.has(flag)) {
      applyBool(flag, svc);
      continue;
    }
    if (UNSUPPORTED_VALUE_FLAGS.has(flag)) {
      // Consume its value (if separate) so it isn't mistaken for the image.
      if (attached === null && i + 1 < tokens.length && !tokens[i + 1].startsWith("-")) i++;
      warnings.push(`${flag} has no simple compose equivalent — add it by hand.`);
      continue;
    }
    if (VALUE_FLAGS.has(flag)) {
      let value = attached;
      if (value === null) {
        if (i + 1 >= tokens.length) {
          warnings.push(`${flag} is missing its value.`);
          continue;
        }
        value = tokens[++i];
      }
      applyValue(flag, value, svc, namedVolumes, externalNetworks, (n) => (name = n));
      continue;
    }
    warnings.push(`Unknown flag ${flag} — skipped.`);
  }

  if (!svc.image) {
    warnings.push("No image found in the command.");
  }
  const serviceName = name ? sanitizeName(name) : sanitizeName(svc.image);
  const yaml = render(serviceName, svc, namedVolumes, externalNetworks);
  return { yaml, serviceName, warnings };
}

function applyBool(flag: string, svc: Service) {
  switch (flag) {
    case "-i": case "--interactive": svc.stdinOpen = true; break;
    case "-t": case "--tty": svc.tty = true; break;
    case "--privileged": svc.privileged = true; break;
    case "--init": svc.init = true; break;
    case "--read-only": svc.readOnly = true; break;
    // -d/--detach and --rm have no compose meaning (compose is detached and
    // manages lifecycle) — intentionally ignored.
  }
}

function applyValue(
  flag: string,
  value: string,
  svc: Service,
  namedVolumes: Set<string>,
  externalNetworks: Set<string>,
  setName: (n: string) => void,
) {
  switch (flag) {
    case "--name": setName(value); break;
    case "-p": case "--publish": svc.ports.push(value); break;
    case "-e": case "--env": svc.environment.push(value); break;
    case "--env-file": svc.envFile.push(value); break;
    case "-v": case "--volume": {
      svc.volumes.push(value);
      const src = value.split(":")[0];
      // A first segment with no path separator and not a relative/absolute path
      // is a named volume — declare it at the top level.
      if (src && !src.includes("/") && !src.startsWith(".") && !src.startsWith("~")) {
        namedVolumes.add(src);
      }
      break;
    }
    case "--restart": svc.restart = value; break;
    case "--network": case "--net": {
      if (value === "host" || value === "none" || value.startsWith("container:")) {
        svc.networkMode = value;
      } else {
        svc.networks.push(value);
        externalNetworks.add(value);
      }
      break;
    }
    case "-w": case "--workdir": svc.workingDir = value; break;
    case "-u": case "--user": svc.user = value; break;
    case "--entrypoint": svc.entrypoint = value; break;
    case "-l": case "--label": svc.labels.push(value); break;
    case "--cap-add": svc.capAdd.push(value); break;
    case "--cap-drop": svc.capDrop.push(value); break;
    case "--device": svc.devices.push(value); break;
    case "-h": case "--hostname": svc.hostname = value; break;
    case "-m": case "--memory": svc.memLimit = value; break;
    case "--cpus": svc.cpus = value; break;
    case "--shm-size": svc.shmSize = value; break;
    case "--stop-signal": svc.stopSignal = value; break;
    case "--add-host": svc.extraHosts.push(value); break;
    case "--dns": svc.dns.push(value); break;
    case "--sysctl": svc.sysctls.push(value); break;
    case "--tmpfs": svc.tmpfs.push(value); break;
    case "--health-cmd": svc.health.test = value; break;
    case "--health-interval": svc.health.interval = value; break;
    case "--health-timeout": svc.health.timeout = value; break;
    case "--health-retries": svc.health.retries = value; break;
    case "--health-start-period": svc.health.start_period = value; break;
  }
}

// yamlScalar quotes a scalar when needed so the output is always valid YAML. We
// double-quote anything that isn't an obviously-safe bareword; numbers/booleans
// that must stay strings (ports, versions) are quoted too.
function yamlScalar(v: string): string {
  if (v === "") return '""';
  // Barewords made of safe chars stay unquoted so images (nginx:alpine) and env
  // (TZ=UTC) read naturally. A ':' or '=' is safe here because these tokens never
  // contain a space (anything with a space fails this test and gets quoted).
  if (/^[A-Za-z0-9_./:=+@-]+$/.test(v) && !/^(true|false|yes|no|on|off|null|~)$/i.test(v)) {
    // Quote anything that looks like a number so it stays a string.
    if (/^-?\d+(\.\d+)?$/.test(v)) return quote(v);
    return v;
  }
  return quote(v);
}

// quote double-quotes and escapes a scalar — always valid YAML.
function quote(v: string): string {
  return `"${v.replace(/\\/g, "\\\\").replace(/"/g, '\\"')}"`;
}

function render(
  name: string,
  svc: Service,
  namedVolumes: Set<string>,
  externalNetworks: Set<string>,
): string {
  const L: string[] = [];
  L.push("# Generated from a `docker run` command by HiveDock.");
  L.push("# Review it, then Save and Deploy.");
  L.push("services:");
  L.push(`  ${name}:`);
  L.push(`    image: ${yamlScalar(svc.image || "IMAGE_MISSING")}`);
  if (svc.restart) L.push(`    restart: ${yamlScalar(svc.restart)}`);
  else L.push(`    restart: unless-stopped`);
  if (svc.networkMode) L.push(`    network_mode: ${yamlScalar(svc.networkMode)}`);
  if (svc.hostname) L.push(`    hostname: ${yamlScalar(svc.hostname)}`);
  if (svc.user) L.push(`    user: ${yamlScalar(svc.user)}`);
  if (svc.workingDir) L.push(`    working_dir: ${yamlScalar(svc.workingDir)}`);
  if (svc.entrypoint) L.push(`    entrypoint: ${yamlScalar(svc.entrypoint)}`);
  if (svc.privileged) L.push(`    privileged: true`);
  if (svc.init) L.push(`    init: true`);
  if (svc.readOnly) L.push(`    read_only: true`);
  if (svc.stdinOpen) L.push(`    stdin_open: true`);
  if (svc.tty) L.push(`    tty: true`);
  if (svc.memLimit) L.push(`    mem_limit: ${yamlScalar(svc.memLimit)}`);
  if (svc.cpus) L.push(`    cpus: ${yamlScalar(svc.cpus)}`);
  if (svc.shmSize) L.push(`    shm_size: ${yamlScalar(svc.shmSize)}`);
  if (svc.stopSignal) L.push(`    stop_signal: ${yamlScalar(svc.stopSignal)}`);

  // Ports are always quoted so "08:80"-style values can't be read as YAML
  // sexagesimals — the standard compose convention.
  if (svc.ports.length > 0) {
    L.push(`    ports:`);
    for (const p of svc.ports) L.push(`      - ${quote(p)}`);
  }
  block(L, "environment", svc.environment);
  block(L, "env_file", svc.envFile);
  block(L, "volumes", svc.volumes);
  block(L, "labels", svc.labels);
  block(L, "cap_add", svc.capAdd);
  block(L, "cap_drop", svc.capDrop);
  block(L, "devices", svc.devices);
  block(L, "networks", svc.networks);
  block(L, "extra_hosts", svc.extraHosts);
  block(L, "dns", svc.dns);
  block(L, "sysctls", svc.sysctls);
  block(L, "tmpfs", svc.tmpfs);

  if (svc.command.length > 0) {
    // Command args are arbitrary CLI tokens (many start with '-'); JSON-quote
    // each so the flow sequence is always unambiguous.
    L.push(`    command: [${svc.command.map((c) => JSON.stringify(c)).join(", ")}]`);
  }

  if (svc.health.test) {
    L.push(`    healthcheck:`);
    L.push(`      test: ["CMD-SHELL", ${yamlScalar(svc.health.test)}]`);
    for (const k of ["interval", "timeout", "retries", "start_period"]) {
      if (svc.health[k]) L.push(`      ${k}: ${yamlScalar(svc.health[k])}`);
    }
  }

  if (namedVolumes.size > 0) {
    L.push("");
    L.push("volumes:");
    for (const v of namedVolumes) L.push(`  ${v}:`);
  }
  if (externalNetworks.size > 0) {
    L.push("");
    L.push("networks:");
    for (const n of externalNetworks) {
      L.push(`  ${n}:`);
      // Most `--network X` runs join a pre-existing network; mark it external so
      // compose doesn't try to create (and rename) it. Flip to false to let this
      // stack own the network.
      L.push(`    external: true`);
    }
  }

  return L.join("\n") + "\n";
}

// block appends a `key:` list under the service when items is non-empty.
function block(L: string[], key: string, items: string[]) {
  if (items.length === 0) return;
  L.push(`    ${key}:`);
  for (const it of items) L.push(`      - ${yamlScalar(it)}`);
}
