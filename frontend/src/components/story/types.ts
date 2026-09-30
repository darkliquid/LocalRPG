// The exported bundle's payload. It is what an export inlines as
// window.__LOCALRPG_STORY__, and it is deliberately independent of the app's API
// types: a bundle is read from a file with no server behind it.
export interface StoryBeat {
  kind: 'narration' | 'speech' | 'scene_card';
  speaker?: string;
  text: string;
  art?: string;
  // portrait is the speaker's face, which a bundle carries as its own copy.
  portrait?: string;
  // audio is the beat's clips in play order, one file per sentence.
  audio?: string[];
  duration: number;
  // player marks the protagonist's own line, which glows on the left.
  player?: boolean;
}

export interface StoryScene {
  location?: string;
  art?: string;
  beats: StoryBeat[];
}

export interface Story {
  game_name: string;
  display_mode?: 'stage_directions' | 'hidden' | 'raw';
  player_portrait?: string;
  scenes: StoryScene[];
}

declare global {
  interface Window {
    __LOCALRPG_STORY__?: Story;
  }
}
