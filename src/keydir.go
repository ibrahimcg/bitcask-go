package main

type KeyDir struct {
	index map[string]*RecordMetadata
}

func NewKeyDir() *KeyDir {
	return &KeyDir{
		index: make(map[string]*RecordMetadata),
	}
}

func (kd *KeyDir) Update(key string, metadata *RecordMetadata) {
	kd.index[key] = metadata
}

func (kd *KeyDir) Get(key string) *RecordMetadata {
	val, ok := kd.index[key]

	if ok {
		return val
	}
	return nil
}

func (kd *KeyDir) Remove(key string) {
	delete(kd.index, key)
}

func (kd *KeyDir) RebuildFromFiles(files []*DataFile) {
}

func (kd *KeyDir) RebuildFromHintFiles(hintFiles []*HintFile) {
}
