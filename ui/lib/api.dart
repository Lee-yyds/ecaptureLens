import 'dart:convert';

import 'package:http/http.dart' as http;

import 'http_client.dart' if (dart.library.io) 'http_client_io.dart';

typedef Json = Map<String, dynamic>;

class LensApi {
  LensApi(this.url, this.token);

  final String url;
  final String token;
  final http.Client _client = createClient();

  Future<dynamic> call(String method, [Json params = const {}]) async {
    final response = await _client.post(
      Uri.parse('$url/api'),
      headers: {
        'Content-Type': 'application/json',
        if (token.isNotEmpty) 'Authorization': 'Bearer $token',
      },
      body: jsonEncode({'method': method, 'params': params}),
    );
    final text = utf8.decode(response.bodyBytes);
    final body = jsonDecode(text) as Json;
    if (body['error'] != null) throw LensError(body['error'].toString());
    if (response.statusCode != 200) {
      throw LensError('HTTP ${response.statusCode}: $text');
    }
    return body['result'];
  }

  void close() => _client.close();
}

class LensError implements Exception {
  LensError(this.message);
  final String message;
  @override
  String toString() => message;
}
