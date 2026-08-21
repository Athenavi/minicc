const statusDot = document.getElementById("status-dot")!;
const statusText = document.getElementById("status-text")!;
const wsUrlInput = document.getElementById("ws-url") as HTMLInputElement;
const btnReconnect = document.getElementById("btn-reconnect")!;
const btnDisconnect = document.getElementById("btn-disconnect")!;
const clientIdSpan = document.getElementById("client-id")!;

chrome.storage.local.get(
  ["rpa_ws_url", "rpa_connection_status", "rpa_client_id"],
  (data) => {
    wsUrlInput.value = data.rpa_ws_url || "ws://localhost:8080/ws/rpa";
    clientIdSpan.textContent = data.rpa_client_id || "-";
    updateStatus(data.rpa_connection_status === "connected");
  }
);

chrome.runtime.sendMessage({ type: "GET_STATUS" }, (response) => {
  if (response) updateStatus(response.connected);
});

function updateStatus(connected: boolean): void {
  statusDot.className = `dot ${connected ? "connected" : "disconnected"}`;
  statusText.textContent = connected ? "Connected" : "Disconnected";
}

btnReconnect.addEventListener("click", () => {
  const url = wsUrlInput.value.trim();
  if (url) {
    chrome.runtime.sendMessage({ type: "SET_WS_URL", url }, () => {
      updateStatus(false);
      statusText.textContent = "Reconnecting...";
    });
  }
});

btnDisconnect.addEventListener("click", () => {
  chrome.runtime.sendMessage({ type: "RECONNECT" }, () => {
    updateStatus(false);
  });
});
