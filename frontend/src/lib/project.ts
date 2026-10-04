import { APIClient } from '../api/client';

// Project links shown in the application menu and About dialog. The repository
// URL matches the one tools/sitegen publishes; the issue tracker is derived from
// it so a rename only has to happen here.
export const PROJECT_NAME = 'LocalRPG';
export const PROJECT_TAGLINE = 'Local-First LLM Tabletop RPG Client';
export const PROJECT_REPO_URL = 'https://github.com/darkliquid/LocalRPG';
export const PROJECT_ISSUES_URL = `${PROJECT_REPO_URL}/issues`;

// openExternal opens a link in the user's default browser.
//
// The Wails webview installs no handler for a new window, so an anchor click is
// a silent no-op in the desktop app. The backend asks the window's Browser
// manager to reach the system browser instead. Browser and socket mode run
// without a window, answer 501, and fall back to opening a tab.
export async function openExternal(url: string) {
  try {
    await APIClient.openURL(url);
    return;
  } catch {
    // No desktop window to ask: open a tab instead.
  }
  openInNewTab(url);
}

function openInNewTab(url: string) {
  const link = document.createElement('a');
  link.href = url;
  link.target = '_blank';
  link.rel = 'noopener noreferrer';
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
}
