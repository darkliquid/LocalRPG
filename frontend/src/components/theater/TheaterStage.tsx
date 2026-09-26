import React from 'react';

interface TheaterStageProps {
  backgroundURL?: string;
  playerPortrait?: string;
  npcPortrait?: string;
  playerLabel?: string;
  npcLabel?: string;
  playerActive?: boolean;
  npcActive?: boolean;
}

// TheaterStage is the visual-novel backdrop: a full-bleed scene with square,
// face-focused character boxes anchored to the left and right edges. Portraits
// stay fully opaque so they are always readable; the active speaker is marked
// with a coloured border and glow instead.
export const TheaterStage: React.FC<TheaterStageProps> = ({
  backgroundURL,
  playerPortrait,
  npcPortrait,
  playerLabel,
  npcLabel,
  playerActive = false,
  npcActive = false,
}) => (
  <>
    <div
      className="absolute inset-0 bg-cover bg-center transition-all duration-700 pointer-events-none"
      style={{
        backgroundImage: backgroundURL
          ? `url(${backgroundURL})`
          : 'radial-gradient(ellipse at center, #261e1b 0%, #0c0a09 100%)',
      }}
    />
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
            <img src={playerPortrait} alt="" aria-hidden="true" className="w-full h-full object-cover object-top" />
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
            <img
              src={npcPortrait}
              alt=""
              aria-hidden="true"
              className="w-full h-full object-cover object-top scale-x-[-1]"
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
