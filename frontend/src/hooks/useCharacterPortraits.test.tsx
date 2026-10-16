import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { useCharacterPortraits } from './useCharacterPortraits';
import { EntitySummary, Turn } from '../types';

function Probe({ chronicle, entities }: { chronicle: Turn[]; entities: EntitySummary[] }) {
  const { portraits, notePortrait } = useCharacterPortraits({ chronicle, entities, activeGameID: 'campaign' });
  return (
    <>
      <button type="button" onClick={() => notePortrait('streamed', { url: '/streamed.png', hasCustom: false })}>
        Stream one
      </button>
      <ul>
        {Object.entries(portraits).map(([id, portrait]) => (
          <li key={id}>{`${id}:${portrait.hasCustom ? 'custom' : 'generated'}:${portrait.url}`}</li>
        ))}
      </ul>
    </>
  );
}

function speechTurn(overrides: Partial<Turn['segments'] extends (infer S)[] | undefined ? S : never>) {
  return [
    {
      turn_number: 1,
      input_text: '',
      mode: '',
      prose: '',
      segments: [{ kind: 'speech', speaker: 'Lady Evelyn', ...overrides }],
    },
  ] as unknown as Turn[];
}

const character: EntitySummary = {
  id: 'evelyn',
  name: 'Lady Evelyn',
  type: 'character',
  has_portrait: true,
  portrait_url: '/api/game/campaign/character/evelyn/portrait',
};

describe('useCharacterPortraits', () => {
  it('takes a generated portrait from a turn segment', () => {
    render(<Probe chronicle={speechTurn({ portrait_url: '/portrait.png' })} entities={[]} />);

    expect(screen.getByText(/lady-evelyn:generated:\/portrait\.png/)).toBeInTheDocument();
  });

  it('takes a custom portrait from the entity list', () => {
    render(<Probe chronicle={[]} entities={[character]} />);

    expect(screen.getByText(/evelyn:custom:/)).toBeInTheDocument();
  });

  it('lets a custom portrait replace a generated one', () => {
    const { rerender } = render(
      <Probe chronicle={speechTurn({ speaker_id: 'evelyn', portrait_url: '/generated.png' })} entities={[]} />,
    );
    expect(screen.getByText(/evelyn:generated:/)).toBeInTheDocument();

    rerender(
      <Probe chronicle={speechTurn({ speaker_id: 'evelyn', portrait_url: '/generated.png' })} entities={[character]} />,
    );

    expect(screen.getByText(/evelyn:custom:/)).toBeInTheDocument();
  });

  it('does not let a later generated portrait downgrade a custom one', () => {
    const { rerender } = render(<Probe chronicle={[]} entities={[character]} />);
    expect(screen.getByText(/evelyn:custom:/)).toBeInTheDocument();

    rerender(
      <Probe chronicle={speechTurn({ speaker_id: 'evelyn', portrait_url: '/generated.png' })} entities={[character]} />,
    );

    expect(screen.getByText(/evelyn:custom:/)).toBeInTheDocument();
    expect(screen.queryByText(/generated/)).not.toBeInTheDocument();
  });

  it('takes a portrait the stream reports mid-turn', async () => {
    render(<Probe chronicle={[]} entities={[]} />);

    screen.getByRole('button', { name: 'Stream one' }).click();

    expect(await screen.findByText(/streamed:generated:\/streamed\.png/)).toBeInTheDocument();
  });

  it('ignores a segment with no portrait and an entity that is not a character', () => {
    render(
      <Probe
        chronicle={speechTurn({ speaker: 'Narrator' })}
        entities={[{ id: 'saltmarch', name: 'Saltmarch', type: 'location', has_portrait: true }]}
      />,
    );

    expect(screen.queryAllByRole('listitem')).toHaveLength(0);
  });
});
