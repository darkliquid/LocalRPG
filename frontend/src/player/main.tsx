import React from 'react';
import { createRoot } from 'react-dom/client';
import { StoryPlayer } from '../components/story/StoryPlayer';
import '../index.css';

// The exported bundle's entry: it reads the story the export inlined and plays it.
// There is no API and no fetch, so a bundle opens from a file.
const story = window.__LOCALRPG_STORY__;
const container = document.getElementById('story-player');

if (container) {
  createRoot(container).render(
    <React.StrictMode>
      {story ? (
        <StoryPlayer story={story} />
      ) : (
        <div className="fixed inset-0 flex items-center justify-center bg-stone-950 text-stone-400 font-sans">
          This page is a story export, and it is missing its story data.
        </div>
      )}
    </React.StrictMode>
  );
}
