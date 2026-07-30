import { describe, it, expect } from "vitest";
import { dockerRunToCompose, tokenize } from "./dockerRun";

describe("tokenize", () => {
  it("splits on whitespace and honors quotes", () => {
    expect(tokenize(`docker run -e "A=b c" nginx`)).toEqual([
      "docker", "run", "-e", "A=b c", "nginx",
    ]);
  });

  it("handles backslash line continuations", () => {
    const cmd = "docker run \\\n  -p 80:80 \\\n  nginx";
    expect(tokenize(cmd)).toEqual(["docker", "run", "-p", "80:80", "nginx"]);
  });

  it("keeps single-quoted content literal", () => {
    expect(tokenize(`run -e 'PASS=a b$c' img`)).toEqual([
      "run", "-e", "PASS=a b$c", "img",
    ]);
  });
});

describe("dockerRunToCompose", () => {
  it("converts a simple run with name, ports, env, volume, restart", () => {
    const { yaml, serviceName, warnings } = dockerRunToCompose(
      "docker run -d --name web -p 8080:80 -e TZ=UTC -v /srv/html:/usr/share/nginx/html --restart unless-stopped nginx:alpine",
    );
    expect(serviceName).toBe("web");
    expect(warnings).toEqual([]);
    expect(yaml).toContain("services:");
    expect(yaml).toContain("  web:");
    expect(yaml).toContain('image: nginx:alpine');
    expect(yaml).toContain('restart: unless-stopped');
    expect(yaml).toContain('- "8080:80"');
    expect(yaml).toContain("- TZ=UTC");
    expect(yaml).toContain("- /srv/html:/usr/share/nginx/html");
  });

  it("derives the service name from the image when --name is absent", () => {
    const { serviceName } = dockerRunToCompose("docker run ghcr.io/org/my-app:1.2 ");
    expect(serviceName).toBe("my-app");
  });

  it("declares a top-level named volume", () => {
    const { yaml } = dockerRunToCompose("docker run -v pgdata:/var/lib/postgresql/data postgres:16");
    expect(yaml).toContain("- pgdata:/var/lib/postgresql/data");
    expect(yaml).toMatch(/\nvolumes:\n {2}pgdata:/);
  });

  it("does not treat a bind mount as a named volume", () => {
    const { yaml } = dockerRunToCompose("docker run -v ./data:/data busybox");
    expect(yaml).not.toMatch(/\nvolumes:\n/);
  });

  it("maps --network host to network_mode", () => {
    const { yaml } = dockerRunToCompose("docker run --network host jellyfin/jellyfin");
    expect(yaml).toContain("network_mode: host");
    expect(yaml).not.toContain("networks:");
  });

  it("maps a custom --network to networks with external: true", () => {
    const { yaml } = dockerRunToCompose("docker run --network proxy nginx");
    expect(yaml).toContain("    networks:");
    expect(yaml).toContain("      - proxy");
    expect(yaml).toMatch(/\nnetworks:\n {2}proxy:\n {4}external: true/);
  });

  it("splits grouped short boolean flags (-it)", () => {
    const { yaml } = dockerRunToCompose("docker run -it --name shell alpine sh");
    expect(yaml).toContain("stdin_open: true");
    expect(yaml).toContain("tty: true");
    expect(yaml).toContain('command: ["sh"]');
  });

  it("captures the container command after the image", () => {
    const { yaml } = dockerRunToCompose("docker run redis:7 redis-server --save 60 1");
    expect(yaml).toContain('command: ["redis-server", "--save", "60", "1"]');
  });

  it("accepts --flag=value form", () => {
    const { yaml, serviceName } = dockerRunToCompose(
      "docker run --name=api --restart=always --env=DEBUG=1 caddy",
    );
    expect(serviceName).toBe("api");
    expect(yaml).toContain("restart: always");
    expect(yaml).toContain("- DEBUG=1");
  });

  it("accepts an attached short value (-eKEY=VAL)", () => {
    const { yaml } = dockerRunToCompose("docker run -eFOO=bar busybox");
    expect(yaml).toContain("- FOO=bar");
  });

  it("builds a healthcheck from --health-* flags", () => {
    const { yaml } = dockerRunToCompose(
      'docker run --health-cmd "curl -f localhost || exit 1" --health-interval 30s --health-retries 3 app',
    );
    expect(yaml).toContain("healthcheck:");
    expect(yaml).toContain('test: ["CMD-SHELL", "curl -f localhost || exit 1"]');
    expect(yaml).toContain("interval: 30s");
    expect(yaml).toContain('retries: "3"');
  });

  it("warns on unsupported flags and still produces valid output", () => {
    const { yaml, warnings } = dockerRunToCompose(
      "docker run --gpus all --log-driver json-file nginx",
    );
    expect(yaml).toContain("image: nginx");
    expect(warnings.some((w) => w.includes("--gpus"))).toBe(true);
    expect(warnings.some((w) => w.includes("--log-driver"))).toBe(true);
  });

  it("warns when there is no image", () => {
    const { warnings } = dockerRunToCompose("docker run -d -p 80:80");
    expect(warnings.some((w) => /no image/i.test(w))).toBe(true);
  });

  it("quotes environment values that contain spaces", () => {
    const { yaml } = dockerRunToCompose('docker run -e "MSG=hello world" busybox');
    expect(yaml).toContain('- "MSG=hello world"');
  });
});
