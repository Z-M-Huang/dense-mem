import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError, ControlApi } from "./api";

describe("ControlApi", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("sends the portal token as a bearer token", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ data: { authenticated: true } }), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    const api = new ControlApi("secret", "/control/api");
    await api.session();

    expect(fetchMock).toHaveBeenCalledWith("/control/api/session", expect.objectContaining({
      headers: expect.objectContaining({ Authorization: "Bearer secret" }),
    }));
  });

  it("throws ApiError with server message", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({ message: "invalid token" }), { status: 401 })));

    const api = new ControlApi("bad", "/control/api");

    await expect(api.session()).rejects.toMatchObject(new ApiError(401, "invalid token"));
  });
});
