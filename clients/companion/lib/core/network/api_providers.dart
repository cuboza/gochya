import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../config/app_config.dart';
import 'gochya_api_client.dart';

final appConfigProvider = Provider<AppConfig>(
  (ref) => AppConfig.fromEnvironment(),
);

final apiClientProvider = Provider<GochyaApiClient>((ref) {
  final client = GochyaApiClient(
    baseUri: ref.watch(appConfigProvider).apiBaseUri,
  );
  ref.onDispose(client.close);
  return client;
});
