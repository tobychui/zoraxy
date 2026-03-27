package uptime

func (m *Monitor) ExportOnlineStatusLog() map[string][]*Record {
	if m == nil {
		return map[string][]*Record{}
	}

	results := make(map[string][]*Record)
	m.logMutex.RLock()
	defer m.logMutex.RUnlock()

	for targetID, records := range m.OnlineStatusLog {
		clonedRecords := make([]*Record, 0, len(records))
		for _, record := range records {
			if record == nil {
				continue
			}
			clonedRecord := *record
			clonedRecords = append(clonedRecords, &clonedRecord)
		}
		results[targetID] = clonedRecords
	}

	return results
}
