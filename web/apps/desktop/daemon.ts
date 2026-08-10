export type DaemonRegistration = {
  instance_id: string;
  version: string;
  url: string;
  pid: number;
  created_at: string;
};

type DaemonReadiness = {
  ready: boolean;
  instance_id: string;
  version: string;
};

export type DaemonTransport = { origin: string; password: string };

function basicAuthorization(password: string): string {
	const bytes = new TextEncoder().encode(`wingman:${password}`);
	let value = "";
	for (const byte of bytes) value += String.fromCharCode(byte);
	return `Basic ${btoa(value)}`;
}

type Dependencies = {
  stateDir: () => string;
  readTextFile: (path: string) => Promise<string>;
  fetch: typeof fetch;
  now?: () => number;
  timeoutSignal?: () => AbortSignal;
};

export function daemonStateDir(getenv: (name: string) => string | undefined): string {
  const state = getenv("XDG_STATE_HOME");
  if (state) return `${state}/wingman`;
  const home = getenv("HOME") ?? getenv("USERPROFILE");
  if (!home) throw new Error("unable to resolve the user home directory");
  return `${home}/.local/state/wingman`;
}

export class DaemonDiscovery {
  private cached: { value: DaemonTransport; expiresAt: number } | undefined;
  private pending: Promise<DaemonTransport> | undefined;
  private readonly now: () => number;
  private readonly timeoutSignal: () => AbortSignal;

  constructor(private readonly deps: Dependencies) {
    this.now = deps.now ?? Date.now;
    this.timeoutSignal = deps.timeoutSignal ?? (() => AbortSignal.timeout(3_000));
  }

  transport(): Promise<DaemonTransport> {
    if (this.cached && this.cached.expiresAt > this.now()) return Promise.resolve(this.cached.value);
    if (this.pending) return this.pending;
    const pending = this.load();
    this.pending = pending;
    return pending.finally(() => {
      if (this.pending === pending) this.pending = undefined;
    });
  }

  invalidate() {
    this.cached = undefined;
  }

  private async load(): Promise<DaemonTransport> {
    const state = this.deps.stateDir();
		const [rawRegistration, rawPassword] = await Promise.all([
			this.deps.readTextFile(`${state}/registration.json`),
			this.deps.readTextFile(`${state}/password`),
    ]);
    const registration = JSON.parse(rawRegistration) as DaemonRegistration;
    const origin = new URL(registration.url);
    if ((origin.protocol !== "http:" && origin.protocol !== "https:") || !registration.instance_id) {
      throw new Error("invalid Wingman daemon registration");
    }
		const password = rawPassword.trim();
		if (!password) throw new Error("invalid Wingman daemon password");

    const response = await this.deps.fetch(`${origin.origin}/ready`, {
			headers: { Authorization: basicAuthorization(password) },
      signal: this.timeoutSignal(),
    });
    if (response.status !== 200) throw new Error(`Wingman daemon is not ready: HTTP ${response.status}`);
    const readiness = await response.json() as DaemonReadiness;
    if (!readiness.ready || readiness.instance_id !== registration.instance_id || readiness.version !== registration.version) {
      throw new Error("Wingman daemon readiness does not match registration");
    }

		const value = { origin: origin.origin, password };
    this.cached = { value, expiresAt: this.now() + 1_000 };
    return value;
  }
}

export async function proxyDaemonRequest(discovery: DaemonDiscovery, request: Request, fetchRequest: typeof fetch = fetch): Promise<Response> {
  const url = new URL(request.url);
  const transport = await discovery.transport();
  const target = new Request(`${transport.origin}${url.pathname}${url.search}`, request);
	target.headers.set("Authorization", basicAuthorization(transport.password));
  try {
    const response = await fetchRequest(target);
    if (response.status === 401 || response.status === 503) discovery.invalidate();
    return response;
  } catch (error) {
    discovery.invalidate();
    throw error;
  }
}
