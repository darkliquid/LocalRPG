import React from 'react';
import ReactDOM from 'react-dom/client';
import { App } from './App';
import { FailuresProvider } from './context/FailuresContext';
import './index.css';

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <FailuresProvider>
      <App />
    </FailuresProvider>
  </React.StrictMode>,
);
