import type { Env, Session } from "./types";
import { upsertProfile } from "./db";

const GITHUB_AUTHORIZE_URL = "https://github.com/login/oauth/authorize";
const GITHUB_TOKEN_URL = "https://github.com/login/oauth/access_token";
const GITHUB_USER_URL = "https://api.github.com/user";
const SESSION_TTL = 60 * 60 * 24 * 30; // 30 days

function randomHex(bytes: number): string {
  const buf = new Uint8Array(bytes);
  crypto.getRandomValues(buf);
  return Array.from(buf)
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("");
}

export async function handleLogin(
  request: Request,
  env: Env,
): Promise<Response> {
  const url = new URL(request.url);
  const redirectPort = url.searchParams.get("redirect_port");
  const port = parseInt(redirectPort || "", 10);
  if (!redirectPort || isNaN(port) || port < 1024 || port > 65535) {
    return new Response(
      JSON.stringify({ error: "redirect_port must be a valid port (1024-65535)" }),
      { status: 400, headers: { "Content-Type": "application/json" } },
    );
  }

  // Encode redirect_port into state so we can recover it in the callback
  const stateRandom = randomHex(16);
  const state = `${stateRandom}:${port}`;

  // Store state in KV for CSRF verification (TTL 5 minutes)
  await env.SESSIONS.put(`oauth_state:${stateRandom}`, "1", {
    expirationTtl: 300,
  });

  const params = new URLSearchParams({
    client_id: env.GITHUB_CLIENT_ID,
    redirect_uri: env.CALLBACK_URL,
    scope: "read:user",
    state,
  });

  return Response.redirect(`${GITHUB_AUTHORIZE_URL}?${params}`, 302);
}

export async function handleCallback(
  request: Request,
  env: Env,
): Promise<Response> {
  const url = new URL(request.url);
  const code = url.searchParams.get("code");
  const state = url.searchParams.get("state");

  if (!code || !state) {
    return new Response(
      JSON.stringify({ error: "Missing code or state" }),
      { status: 400, headers: { "Content-Type": "application/json" } },
    );
  }

  // Parse redirect_port from state
  const parts = state.split(":");
  if (parts.length !== 2) {
    return new Response(
      JSON.stringify({ error: "Invalid state parameter" }),
      { status: 400, headers: { "Content-Type": "application/json" } },
    );
  }
  const stateRandom = parts[0];
  const redirectPort = parseInt(parts[1], 10);
  if (isNaN(redirectPort) || redirectPort < 1024 || redirectPort > 65535) {
    return new Response(
      JSON.stringify({ error: "Invalid redirect port in state" }),
      { status: 400, headers: { "Content-Type": "application/json" } },
    );
  }

  // Verify CSRF state exists in KV, then delete it
  const storedState = await env.SESSIONS.get(`oauth_state:${stateRandom}`);
  if (!storedState) {
    return new Response(
      JSON.stringify({ error: "Invalid or expired OAuth state" }),
      { status: 400, headers: { "Content-Type": "application/json" } },
    );
  }
  await env.SESSIONS.delete(`oauth_state:${stateRandom}`);

  // Exchange code for access token
  const tokenAbort = new AbortController();
  const tokenTimeout = setTimeout(() => tokenAbort.abort(), 10_000);
  let tokenRes: Response;
  try {
    tokenRes = await fetch(GITHUB_TOKEN_URL, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Accept: "application/json",
      },
      body: JSON.stringify({
        client_id: env.GITHUB_CLIENT_ID,
        client_secret: env.GITHUB_CLIENT_SECRET,
        code,
      }),
      signal: tokenAbort.signal,
    });
  } catch (err) {
    if (err instanceof DOMException && err.name === "AbortError") {
      return new Response(
        JSON.stringify({ error: "GitHub token exchange timed out" }),
        { status: 504, headers: { "Content-Type": "application/json" } },
      );
    }
    throw err;
  } finally {
    clearTimeout(tokenTimeout);
  }

  if (!tokenRes.ok) {
    return new Response(
      JSON.stringify({ error: "Failed to exchange code for token" }),
      { status: 502, headers: { "Content-Type": "application/json" } },
    );
  }

  const tokenData = (await tokenRes.json()) as { access_token?: string; error?: string };
  if (!tokenData.access_token) {
    return new Response(
      JSON.stringify({ error: "Failed to obtain access token" }),
      { status: 400, headers: { "Content-Type": "application/json" } },
    );
  }

  // Fetch GitHub user profile
  const userAbort = new AbortController();
  const userTimeout = setTimeout(() => userAbort.abort(), 10_000);
  let userRes: Response;
  try {
    userRes = await fetch(GITHUB_USER_URL, {
      headers: {
        Authorization: `Bearer ${tokenData.access_token}`,
        Accept: "application/json",
        "User-Agent": "RandomChat",
      },
      signal: userAbort.signal,
    });
  } catch (err) {
    if (err instanceof DOMException && err.name === "AbortError") {
      return new Response(
        JSON.stringify({ error: "GitHub user fetch timed out" }),
        { status: 504, headers: { "Content-Type": "application/json" } },
      );
    }
    throw err;
  } finally {
    clearTimeout(userTimeout);
  }

  if (!userRes.ok) {
    return new Response(
      JSON.stringify({ error: "Failed to fetch GitHub profile" }),
      { status: 502, headers: { "Content-Type": "application/json" } },
    );
  }

  const user = (await userRes.json()) as {
    id: number;
    login: string;
    avatar_url: string;
    bio: string | null;
    public_repos: number;
    created_at: string;
  };

  // Fetch repos for top languages and starred repo
  let topLanguages = "";
  let topRepo = "";
  let topRepoStars = 0;
  try {
    const repoRes = await fetch(
      `https://api.github.com/user/repos?sort=stars&per_page=10&type=owner`,
      {
        headers: {
          Authorization: `Bearer ${tokenData.access_token}`,
          Accept: "application/json",
          "User-Agent": "RandomChat",
        },
      },
    );
    if (repoRes.ok) {
      const repos = (await repoRes.json()) as Array<{
        name: string;
        language: string | null;
        stargazers_count: number;
        fork: boolean;
      }>;
      const owned = repos.filter((r) => !r.fork);
      // Top starred repo
      if (owned.length > 0) {
        topRepo = owned[0].name;
        topRepoStars = owned[0].stargazers_count;
      }
      // Top languages
      const langCount = new Map<string, number>();
      for (const r of owned) {
        if (r.language) {
          langCount.set(r.language, (langCount.get(r.language) || 0) + 1);
        }
      }
      topLanguages = Array.from(langCount.entries())
        .sort((a, b) => b[1] - a[1])
        .slice(0, 3)
        .map(([lang]) => lang)
        .join(",");
    }
  } catch {
    // Non-critical, ignore
  }

  // Fetch contributions via GraphQL
  let contributions = 0;
  let contributionGraph = "";
  try {
    const gqlRes = await fetch("https://api.github.com/graphql", {
      method: "POST",
      headers: {
        Authorization: `Bearer ${tokenData.access_token}`,
        "Content-Type": "application/json",
        "User-Agent": "RandomChat",
      },
      body: JSON.stringify({
        query: `query($login: String!) { user(login: $login) { contributionsCollection { contributionCalendar { totalContributions weeks { contributionDays { contributionCount } } } } } }`,
        variables: { login: user.login },
      }),
    });
    if (gqlRes.ok) {
      const gql = (await gqlRes.json()) as {
        data?: {
          user?: {
            contributionsCollection?: {
              contributionCalendar?: {
                totalContributions?: number;
                weeks?: Array<{
                  contributionDays: Array<{ contributionCount: number }>;
                }>;
              };
            };
          };
        };
      };
      const cal = gql.data?.user?.contributionsCollection?.contributionCalendar;
      contributions = cal?.totalContributions ?? 0;
      // Encode last 12 weeks as compact string: each day = level 0-4
      if (cal?.weeks) {
        const last12 = cal.weeks.slice(-12);
        const levels: string[] = [];
        for (const week of last12) {
          for (const day of week.contributionDays) {
            const c = day.contributionCount;
            const level = c === 0 ? 0 : c <= 2 ? 1 : c <= 5 ? 2 : c <= 9 ? 3 : 4;
            levels.push(String(level));
          }
        }
        contributionGraph = levels.join("");
      }
    }
  } catch {
    // Non-critical, ignore
  }

  // Revoke GitHub access token (no longer needed)
  try {
    await fetch(`https://api.github.com/applications/${env.GITHUB_CLIENT_ID}/token`, {
      method: "DELETE",
      headers: {
        Authorization: `Basic ${btoa(`${env.GITHUB_CLIENT_ID}:${env.GITHUB_CLIENT_SECRET}`)}`,
        Accept: "application/json",
        "User-Agent": "RandomChat",
      },
      body: JSON.stringify({ access_token: tokenData.access_token }),
    });
  } catch {
    // Non-critical
  }

  // Upsert profile in D1
  await upsertProfile(env.DB, user.id, user.login, user.avatar_url, user.bio || "", user.public_repos, user.created_at, topLanguages, topRepo, topRepoStars, contributions, contributionGraph);

  // Create session in KV
  const token = crypto.randomUUID();
  const session: Session = {
    github_id: user.id,
    username: user.login,
    avatar_url: user.avatar_url,
    bio: user.bio || "",
    public_repos: user.public_repos,
    github_created_at: user.created_at,
    top_languages: topLanguages,
    top_repo: topRepo,
    top_repo_stars: topRepoStars,
    contributions,
    contribution_graph: contributionGraph,
  };

  await env.SESSIONS.put(`session:${token}`, JSON.stringify(session), {
    expirationTtl: SESSION_TTL,
  });

  // Redirect to CLI localhost
  return Response.redirect(
    `http://localhost:${redirectPort}/callback?token=${token}`,
    302,
  );
}

export async function handleMe(
  request: Request,
  env: Env,
): Promise<Response> {
  const session = await verifySessionFromRequest(request, env);
  if (!session) {
    return new Response(
      JSON.stringify({ error: "Unauthorized" }),
      { status: 401, headers: { "Content-Type": "application/json" } },
    );
  }

  return new Response(JSON.stringify(session), {
    headers: { "Content-Type": "application/json" },
  });
}

export async function handleLogout(
  request: Request,
  env: Env,
): Promise<Response> {
  const token = extractToken(request);
  if (!token) {
    return new Response(
      JSON.stringify({ error: "Unauthorized" }),
      { status: 401, headers: { "Content-Type": "application/json" } },
    );
  }

  await env.SESSIONS.delete(`session:${token}`);

  return new Response(JSON.stringify({ ok: true }), {
    headers: { "Content-Type": "application/json" },
  });
}

export async function verifySession(
  token: string,
  env: Env,
): Promise<Session | null> {
  const raw = await env.SESSIONS.get(`session:${token}`);
  if (!raw) return null;
  return JSON.parse(raw) as Session;
}

function extractToken(request: Request): string | null {
  const auth = request.headers.get("Authorization");
  if (!auth) return null;
  const parts = auth.split(" ");
  if (parts.length !== 2 || parts[0] !== "Bearer") return null;
  return parts[1];
}

async function verifySessionFromRequest(
  request: Request,
  env: Env,
): Promise<Session | null> {
  const token = extractToken(request);
  if (!token) return null;
  return verifySession(token, env);
}
