import { describe, expect, it } from 'vitest';
import { TurnStreamProcessor } from './turnStreamProcessor';

describe('TurnStreamProcessor', () => {
  it('filters a control line out entirely', () => {
    const processor = new TurnStreamProcessor();
    processor.feedChunk('@roll might\nYou shove the door.');
    const segments = processor.getSegments();
    expect(segments.some((segment) => segment.text.includes('@roll'))).toBe(false);
    expect(segments.some((segment) => segment.text.includes('You shove the door.'))).toBe(true);
  });

  it('parses a blockquoted speech line into a speaker and text', () => {
    const processor = new TurnStreamProcessor();
    processor.feedChunk('> Garrick: "Keep moving."\n');
    expect(processor.getSegments()).toContainEqual({ kind: 'speech', speaker: 'Garrick', text: 'Keep moving.' });
  });

  it('streams narration a character at a time', () => {
    const processor = new TurnStreamProcessor();
    processor.feedChunk('The quay');
    expect(processor.getSegments()).toContainEqual({ kind: 'narration', text: 'The quay' });
    processor.feedChunk(' is quiet.');
    expect(processor.getSegments()).toContainEqual({ kind: 'narration', text: 'The quay is quiet.' });
  });

  it('enriches a live speech segment with a canonical speaker id', () => {
    const processor = new TurnStreamProcessor();
    processor.feedChunk('> Garrick: "Keep moving."\n');
    processor.feedSegment({ kind: 'speech', speaker: 'Garrick', speaker_id: 'garrick', text: 'Keep moving.' });
    const speech = processor.getSegments().find((segment) => segment.kind === 'speech');
    expect(speech?.speaker_id).toBe('garrick');
  });

  it('joins wrapped narration lines into one paragraph', () => {
    const processor = new TurnStreamProcessor();
    processor.feedChunk('The quay is quiet\nand the tide is out.\n');
    expect(processor.getSegments()).toContainEqual({
      kind: 'narration',
      text: 'The quay is quiet\nand the tide is out.',
    });
  });
});
