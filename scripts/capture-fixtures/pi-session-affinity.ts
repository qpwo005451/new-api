import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";

export default function (pi: ExtensionAPI) {
  pi.on("before_provider_headers", (event, ctx) => {
    const sessionID = ctx.sessionManager.getSessionId();
    if (!sessionID) {
      return;
    }
    event.headers["X-NewAPI-Session-ID"] = sessionID;
  });
}
