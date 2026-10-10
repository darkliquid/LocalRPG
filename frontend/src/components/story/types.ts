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
  // duration is the pace the script was compiled with; reading is what a viewer needs
  // to read the line. A beat is held for the longer of the two, plus a buffer, so
  // audio can never shorten it.
  duration: number;
  reading?: number;
  // player marks the protagonist's own line, which glows on the left.
  player?: boolean;
  // outcome is the turn's resolved outcome, which the player maps to a mood tint.
  outcome?: string;
}

export interface StoryScene {
  location?: string;
  art?: string;
  beats: StoryBeat[];
  // weather is the location's weather, which the player draws as an overlay.
  weather?: string;
}

export interface Story {
  game_name: string;
  display_mode?: 'stage_directions' | 'hidden' | 'raw';
  // banner is the campaign's own image, which the theatre shows behind a scene that
  // has no art of its own.
  banner?: string;
  player_portrait?: string;
  // player_name labels the protagonist's portrait, as the theatre's does.
  player_name?: string;
  // captions is the story's WebVTT subtitle track, carried in the bundle.
  captions?: string;
  scenes: StoryScene[];
}

declare global {
  interface Window {
    __LOCALRPG_STORY__?: Story;
  }
}
