export type Profile = {
  id: string;
  name: string;
  description: string;
  metadata: Record<string, unknown> | null;
  config: Record<string, unknown> | null;
  created_at: string;
  updated_at: string;
};

export type ApiKey = {
  id: string;
  profile_id: string;
  key_suffix: string | null;
  rate_limit: number;
  last_used_at: string | null;
  expires_at: string | null;
  created_at: string;
};

export type Pagination = {
  limit: number;
  offset: number;
  total: number;
};

export type Page<T> = {
  data: T[];
  pagination: Pagination;
};

export type CreateProfileInput = {
  name: string;
  description: string;
};

export type UpdateProfileInput = CreateProfileInput;

export type CreateApiKeyInput = {
  rate_limit: number;
  expires_at?: string;
};

export type CreatedApiKey = {
  api_key: string;
  key: ApiKey;
};

export class ApiError extends Error {
  status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

type RequestOptions = {
  method?: string;
  body?: unknown;
};

export class ControlApi {
  private readonly token: string;
  private readonly baseUrl: string;

  constructor(token: string, baseUrl = "/control/api") {
    this.token = token;
    this.baseUrl = baseUrl;
  }

  session(): Promise<{ authenticated: boolean }> {
    return this.request<{ authenticated: boolean }>("/session");
  }

  listProfiles(): Promise<Page<Profile>> {
    return this.request<Page<Profile>>("/profiles");
  }

  createProfile(input: CreateProfileInput): Promise<Profile> {
    return this.requestEnvelope<Profile>("/profiles", { method: "POST", body: input });
  }

  updateProfile(profileId: string, input: UpdateProfileInput): Promise<Profile> {
    return this.requestEnvelope<Profile>(`/profiles/${profileId}`, { method: "PATCH", body: input });
  }

  deleteProfile(profileId: string): Promise<{ status: string }> {
    return this.requestEnvelope<{ status: string }>(`/profiles/${profileId}`, { method: "DELETE" });
  }

  listApiKeys(profileId: string): Promise<Page<ApiKey>> {
    return this.request<Page<ApiKey>>(`/profiles/${profileId}/api-keys`);
  }

  createApiKey(profileId: string, input: CreateApiKeyInput): Promise<CreatedApiKey> {
    return this.requestEnvelope<CreatedApiKey>(`/profiles/${profileId}/api-keys`, { method: "POST", body: input });
  }

  deleteApiKey(profileId: string, keyId: string): Promise<{ status: string }> {
    return this.requestEnvelope<{ status: string }>(`/profiles/${profileId}/api-keys/${keyId}`, { method: "DELETE" });
  }

  private async requestEnvelope<T>(path: string, options: RequestOptions = {}): Promise<T> {
    const payload = await this.request<{ data: T }>(path, options);
    return payload.data;
  }

  private async request<T>(path: string, options: RequestOptions = {}): Promise<T> {
    const response = await fetch(`${this.baseUrl}${path}`, {
      method: options.method ?? "GET",
      headers: {
        Authorization: `Bearer ${this.token}`,
        "Content-Type": "application/json",
      },
      body: options.body === undefined ? undefined : JSON.stringify(options.body),
    });

    const text = await response.text();
    const payload = text ? JSON.parse(text) : null;

    if (!response.ok) {
      throw new ApiError(response.status, errorMessage(payload, response.statusText));
    }

    return payload as T;
  }
}

function errorMessage(payload: unknown, fallback: string): string {
  if (payload && typeof payload === "object") {
    const obj = payload as Record<string, unknown>;
    if (typeof obj.message === "string") {
      return obj.message;
    }
    if (typeof obj.error === "string") {
      return obj.error;
    }
  }
  return fallback || "Request failed";
}
