import React from 'react';
import ReactDOM from 'react-dom/client';
import { App } from './App';
import { FailuresProvider } from './context/FailuresContext';
import { SettingsProvider } from './context/SettingsContext';
import './index.css';

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <FailuresProvider>
      {/* The settings are fetched once for the whole app, and the studio edits a
          copy of what every other surface is already reading. */}
      <SettingsProvider>
        <App />
      </SettingsProvider>
    </FailuresProvider>
  </React.StrictMode>,
);
