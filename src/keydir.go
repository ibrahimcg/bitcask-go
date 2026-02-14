package main

type KeyDir struct {
	index map[string]*RecordMetadata
}

func (kd *KeyDir) Update(key string, metadata *RecordMetadata) {
}

func (kd *KeyDir) Get(key string) *RecordMetadata {
	return nil
}

func (kd *KeyDir) Remove(key string) {
}

func (kd *KeyDir) RebuildFromFiles(files []*DataFile) {
}

func (kd *KeyDir) RebuildFromHintFiles(hintFiles []*HintFile) {
}
