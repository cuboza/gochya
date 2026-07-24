class AppConfig {
  const AppConfig({required this.apiBaseUri});

  factory AppConfig.fromEnvironment() {
    return AppConfig(
      apiBaseUri: Uri.parse(
        const String.fromEnvironment(
          'GOCHYA_API_BASE_URL',
          defaultValue: 'https://api.gochya.invalid',
        ),
      ),
    );
  }

  final Uri apiBaseUri;
}
