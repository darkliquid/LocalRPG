import { MechanicsEditor } from '../../MechanicsEditor';
import { MechanicsSpec } from '../../../types';

export interface SystemMechanicsTabProps {
  mechanics: MechanicsSpec;
  setMechanics: (mechanics: MechanicsSpec) => void;
  markDirty: () => void;
}

// SystemMechanicsTab edits the system's declarative mechanics block: the stats,
// tracks and hooks a campaign's turns are resolved against.
export function SystemMechanicsTab({ mechanics, setMechanics, markDirty }: SystemMechanicsTabProps) {
  return (
    <div className="flex-1 overflow-y-auto min-h-0 pr-2">
      <MechanicsEditor
        mechanics={mechanics}
        onChange={(next) => {
          setMechanics(next);
          markDirty();
        }}
      />
    </div>
  );
}
