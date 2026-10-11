import React from 'react';
import { kenBurns, moodTint, parallaxOffset, weatherOverlay } from '../../lib/effects';
import { EntityAvatar } from '../EntityAvatar';

export interface TheaterLayer {
  depth: number;
  url: string;
}

interface TheaterStageProps {
  backgroundURL?: string;
  playerPortrait?: string;
  npcPortrait?: string;
  playerLabel?: string;
  npcLabel?: string;
  playerActive?: boolean;
  npcActive?: boolean;
  // progress is how far through the current beat the stage is, 0 to 1. It drives
  // the Ken Burns transform and the parallax offsets.
  progress?: number;
  // seed chooses the Ken Burns pan direction, so consecutive beats do not all
  // drift the same way.
  seed?: number;
  // outcome is the turn's outcome, which maps to a mood tint.
  outcome?: string;
  // weather is the scene's weather, which maps to an overlay.
  weather?: string;
  // layers is a layered scene's art, ordered back to front. A scene with no
  // layers draws its background flat.
  layers?: TheaterLayer[];
  // reducedMotion disables the Ken Burns transform and the parallax, but leaves
  // the tint and the overlay in place.
  reducedMotion?: boolean;
}

const gradient = 'radial-gradient(ellipse at center, #261e1b 0%, #0c0a09 100%)';

// TheaterStage is the visual-novel backdrop: a full-bleed scene with square,
// face-focused character boxes anchored to the left and right edges. Portraits
// stay fully opaque so they are always readable; the active speaker is marked
// with a coloured border and glow instead.
//
// A beat's image drifts slowly (Ken Burns), a layered scene parallaxes, the
// turn's outcome tints the picture, and the scene's weather draws an overlay.
// Every effect is computed from the beat's progress, so the export's renderer
// draws the same stage.
export const TheaterStage: React.FC<TheaterStageProps> = ({
  backgroundURL,
  playerPortrait,
  npcPortrait,
  playerLabel,
  npcLabel,
  playerActive = false,
  npcActive = false,
  progress = 0,
  seed = 0,
  outcome,
  weather,
  layers,
  reducedMotion = false,
}) => {
  const motion = !reducedMotion;
  const kb = motion ? kenBurns(seed, progress) : { scale: 1, dx: 0, dy: 0 };
  const tint = moodTint(outcome);
  const overlay = weatherOverlay(weather);

  const backgroundTransform = (extraDx = 0) =>
    motion ? `scale(${kb.scale}) translate(${(kb.dx + extraDx) * 100}%, ${kb.dy * 100}%)` : undefined;

  return (
    <>
      {layers && layers.length > 0 ? (
        layers.map((layer, index) => (
          <div
            key={`${layer.depth}-${index}`}
            data-stage="layer"
            data-layer-depth={layer.depth}
            className="absolute inset-0 bg-cover bg-center pointer-events-none transition-transform duration-700 ease-linear"
            style={{
              backgroundImage: `url(${layer.url})`,
              transform: backgroundTransform(parallaxOffset(layer.depth, progress)),
              transformOrigin: 'center',
            }}
          />
        ))
      ) : (
        <div
          data-stage="background"
          className="absolute inset-0 bg-cover bg-center pointer-events-none transition-transform duration-700 ease-linear"
          style={{
            backgroundImage: backgroundURL ? `url(${backgroundURL})` : gradient,
            transform: backgroundURL ? backgroundTransform() : undefined,
            transformOrigin: 'center',
          }}
        />
      )}

      {tint.opacity > 0 && (
        <div
          data-stage="tint"
          data-tint={tint.color}
          className="absolute inset-0 pointer-events-none"
          style={{ backgroundColor: tint.color, opacity: tint.opacity }}
        />
      )}

      {overlay && (
        <div
          data-stage="weather"
          data-weather={overlay.kind}
          className={`absolute inset-0 pointer-events-none theater-weather theater-weather-${overlay.kind}`}
          aria-hidden="true"
        />
      )}

      <div className="absolute inset-0 bg-gradient-to-t from-black/90 via-black/45 to-black/60 pointer-events-none" />

      <div className="absolute inset-x-0 top-20 bottom-[30vh] z-10 flex items-end justify-between px-[4vw] pointer-events-none">
        {playerPortrait ? (
          <div className="flex flex-col items-center gap-2">
            <div
              className={`aspect-square w-[clamp(140px,24vw,340px)] overflow-hidden rounded-2xl border-2 bg-stone-900 shadow-2xl transition-all duration-300 ${
                playerActive
                  ? 'border-sky-400 shadow-[0_10px_40px_rgba(56,189,248,0.55)]'
                  : 'border-white/30'
              }`}
            >
              <EntityAvatar
                src={playerPortrait}
                name={playerLabel ?? 'Player'}
                alt=""
                className="w-full h-full object-cover object-top rounded-2xl"
              />
            </div>
            {playerLabel && (
              <span
                className={`px-3 py-1 rounded-full text-xs font-sans font-bold tracking-wide border shadow-lg ${
                  playerActive ? 'bg-sky-600 text-white border-sky-300/60' : 'bg-black/60 text-stone-200 border-white/20'
                }`}
              >
                {playerLabel}
              </span>
            )}
          </div>
        ) : (
          <span />
        )}
        {npcPortrait ? (
          <div className="flex flex-col items-center gap-2">
            <div
              className={`aspect-square w-[clamp(140px,24vw,340px)] overflow-hidden rounded-2xl border-2 bg-stone-900 shadow-2xl transition-all duration-300 ${
                npcActive
                  ? 'border-purple-400 shadow-[0_10px_40px_rgba(168,85,247,0.55)]'
                  : 'border-white/30'
              }`}
            >
              <EntityAvatar
                src={npcPortrait}
                name={npcLabel ?? 'Unknown'}
                alt=""
                className="w-full h-full object-cover object-top scale-x-[-1] rounded-2xl"
              />
            </div>
            {npcLabel && (
              <span
                className={`px-3 py-1 rounded-full text-xs font-sans font-bold tracking-wide border shadow-lg ${
                  npcActive
                    ? 'bg-purple-600 text-white border-purple-300/60'
                    : 'bg-black/60 text-stone-200 border-white/20'
                }`}
              >
                {npcLabel}
              </span>
            )}
          </div>
        ) : (
          <span />
        )}
      </div>
    </>
  );
};
