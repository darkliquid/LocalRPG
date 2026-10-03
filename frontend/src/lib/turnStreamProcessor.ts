import { TurnSegment } from '../types';

/**
 * TurnStreamProcessor processes streaming turn output deltas in real-time.
 *
 * It guarantees that:
 * 1. Control statements starting with '@' (e.g. @roll, @persona) are completely
 *    filtered out so they never appear in the chronicle as plain text.
 * 2. Blockquoted speech lines starting with '>' (e.g. > Speaker: "utterance")
 *    are parsed live into speech segments.
 * 3. Normal narration streams into narration segments live character-by-character.
 * 4. Canonical segment events from the backend enrich segments with resolved speaker IDs.
 */
export class TurnStreamProcessor {
  private segments: TurnSegment[] = [];
  private currentLine = '';
  private currentParagraph = '';
  private isControl = false;
  private isSpeech = false;

  public reset(): void {
    this.segments = [];
    this.currentLine = '';
    this.currentParagraph = '';
    this.isControl = false;
    this.isSpeech = false;
  }

  public getSegments(): TurnSegment[] {
    return this.buildDisplaySegments();
  }

  public feedChunk(chunk: string): TurnSegment[] {
    for (let i = 0; i < chunk.length; i++) {
      const char = chunk[i];
      if (char === '\r') continue;

      if (char === '\n') {
        this.finishLine();
      } else {
        this.currentLine += char;
        // Determine line type on the first non-whitespace character
        if (!this.isControl && !this.isSpeech && this.currentLine.trim().length > 0) {
          const trimmed = this.currentLine.trimStart();
          if (trimmed.startsWith('@')) {
            this.isControl = true;
          } else if (trimmed.startsWith('>')) {
            this.isSpeech = true;
          }
        }
      }
    }
    return this.buildDisplaySegments();
  }

  public feedSegment(segment: TurnSegment): TurnSegment[] {
    if (segment.kind === 'speech') {
      // Find matching live speech segment to enrich with canonical metadata (speaker_id)
      for (let i = this.segments.length - 1; i >= 0; i--) {
        if (
          this.segments[i].kind === 'speech' &&
          (!segment.speaker || this.segments[i].speaker === segment.speaker)
        ) {
          this.segments[i] = {
            ...this.segments[i],
            ...segment,
          };
          return this.buildDisplaySegments();
        }
      }

      if (this.isSpeech) {
        this.isSpeech = false;
        this.currentLine = '';
      }
      this.segments.push(segment);
      return this.buildDisplaySegments();
    }

    if (segment.kind === 'narration') {
      if (this.segments.length > 0 && this.segments[this.segments.length - 1].kind === 'narration') {
        this.segments[this.segments.length - 1] = {
          ...this.segments[this.segments.length - 1],
          ...segment,
        };
        return this.buildDisplaySegments();
      }
      if (this.currentParagraph.trim() === segment.text.trim()) {
        this.currentParagraph = '';
      }
      this.segments.push(segment);
      return this.buildDisplaySegments();
    }

    return this.buildDisplaySegments();
  }

  private finishLine(): void {
    const trimmed = this.currentLine.trim();

    if (this.isControl) {
      this.isControl = false;
      this.currentLine = '';
      return;
    }

    if (this.isSpeech) {
      this.flushCurrentParagraph();
      const body = this.currentLine.trimStart().replace(/^>\s*/, '');
      const parsed = this.parseSpeechBody(body);
      if (parsed.speaker || parsed.text) {
        this.segments.push({
          kind: 'speech',
          speaker: parsed.speaker,
          text: parsed.text,
        });
      }
      this.isSpeech = false;
      this.currentLine = '';
      return;
    }

    if (trimmed === '') {
      this.flushCurrentParagraph();
      this.currentLine = '';
      return;
    }

    if (this.currentParagraph.length > 0) {
      this.currentParagraph += '\n' + this.currentLine;
    } else {
      this.currentParagraph = this.currentLine;
    }
    this.currentLine = '';
  }

  private flushCurrentParagraph(): void {
    const text = this.currentParagraph.trim();
    if (text.length > 0) {
      this.segments.push({
        kind: 'narration',
        text,
      });
    }
    this.currentParagraph = '';
  }

  private parseSpeechBody(body: string): { speaker: string; text: string } {
    const colonIdx = body.indexOf(':');
    if (colonIdx === -1) {
      return { speaker: body.trim(), text: '' };
    }
    const speaker = body.slice(0, colonIdx).trim();
    let text = body.slice(colonIdx + 1).trim();
    if (text.startsWith('"') || text.startsWith('“') || text.startsWith('‘')) {
      text = text.slice(1);
    }
    if (text.endsWith('"') || text.endsWith('”') || text.endsWith('’')) {
      text = text.slice(0, -1);
    }
    return { speaker, text: text.trim() };
  }

  private buildDisplaySegments(): TurnSegment[] {
    const result = [...this.segments];

    if (this.isControl) {
      if (this.currentParagraph.trim().length > 0) {
        result.push({
          kind: 'narration',
          text: this.currentParagraph.trim(),
        });
      }
      return result;
    }

    if (this.isSpeech) {
      if (this.currentParagraph.trim().length > 0) {
        result.push({
          kind: 'narration',
          text: this.currentParagraph.trim(),
        });
      }
      const body = this.currentLine.trimStart().replace(/^>\s*/, '');
      const parsed = this.parseSpeechBody(body);
      if (parsed.speaker || parsed.text) {
        result.push({
          kind: 'speech',
          speaker: parsed.speaker,
          text: parsed.text,
        });
      }
      return result;
    }

    const fullText = this.currentParagraph
      ? (this.currentLine ? `${this.currentParagraph}\n${this.currentLine}` : this.currentParagraph)
      : this.currentLine;

    const trimmed = fullText.trim();
    if (trimmed.length > 0) {
      result.push({
        kind: 'narration',
        text: trimmed,
      });
    }

    return result;
  }
}
