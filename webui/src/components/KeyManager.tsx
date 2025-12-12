import React from 'react';
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Trash2, Plus } from 'lucide-react';

export interface KeyConfig {
  term: string;
  remark: string;
  status: boolean;
}

interface KeyManagerProps {
  keys: KeyConfig[];
  onChange: (newKeys: KeyConfig[]) => void;
}

const KeyManager: React.FC<KeyManagerProps> = ({ keys, onChange }) => {
  const activeKeysCount = keys.filter(k => k.status && k.term.trim()).length;

  const handleAddKey = () => {
    onChange([...keys, { term: '', remark: '', status: true }]);
  };

  const handleRemoveKey = (index: number) => {
    const newKeys = keys.filter((_, i) => i !== index);
    onChange(newKeys);
  };

  const handleKeyChange = (index: number, field: keyof KeyConfig, value: string | boolean) => {
    const newKeys = keys.map((key, i) =>
      i === index ? { ...key, [field]: value } : key
    );
    onChange(newKeys);
  };

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <Label className="text-sm font-medium">API Keys ({keys.length})</Label>
        <Button type="button" variant="ghost" size="sm" onClick={handleAddKey}>
          <Plus className="h-4 w-4" />
        </Button>
      </div>
      {activeKeysCount === 0 && keys.length > 0 && (
        <span className="text-xs text-destructive">至少需要一个启用的Key</span>
      )}
      {keys.length === 0 ? (
        <p className="text-sm text-muted-foreground">点击 + 添加 API Key</p>
      ) : (
        <div className="space-y-2 max-h-60 overflow-y-auto">
          {keys.map((key, index) => (
            <div key={index} className="flex items-center gap-2">
              <Input
                placeholder="API Key"
                value={key.term}
                onChange={(e) => handleKeyChange(index, 'term', e.target.value)}
                className="flex-1"
              />
              <Input
                placeholder="备注"
                value={key.remark}
                onChange={(e) => handleKeyChange(index, 'remark', e.target.value)}
                className="w-32"
              />
              <Switch
                checked={key.status}
                onCheckedChange={(checked) => handleKeyChange(index, 'status', checked)}
              />
              <Button
                type="button"
                variant="ghost"
                size="icon"
                onClick={() => handleRemoveKey(index)}
              >
                <Trash2 className="h-4 w-4" />
              </Button>
            </div>
          ))}
        </div>
      )}
    </div>
  );
};

export default KeyManager;
