class AppConfig {
  const AppConfig({required this.apiBaseUri, this.googleServerClientId = ''});

  factory AppConfig.fromEnvironment() {
    return AppConfig(
      apiBaseUri: Uri.parse(
        const String.fromEnvironment(
          'GOCHYA_API_BASE_URL',
          defaultValue: 'https://api.gochya.invalid',
        ),
      ),
      googleServerClientId: const String.fromEnvironment(
        'GOCHYA_GOOGLE_SERVER_CLIENT_ID',
      ),
    );
  }

  final Uri apiBaseUri;
  final String googleServerClientId;
}
