package service

// Only the production Wire provider starts the scheduler. Unit tests construct
// an inert service, so loading a fixture can never initiate provider requests.
func ProvideModelEvaluationService(repo ModelEvaluationRepository, settings SettingRepository, groups GroupRepository, encryptor SecretEncryptor) *ModelEvaluationService {
	s := NewModelEvaluationService(repo, settings, groups, encryptor)
	s.Start()
	return s
}
