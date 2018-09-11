<?php

require 'vendor/autoload.php';

use Slim\Http\Request;
use Slim\Http\Response;

$app = new \Slim\App();

$staticFileLoad = function (Request $request, Response $response, callable $next) {
    if (!$request->isGet()) {
        return $next($request, $response);
    }

    $file = $request->getUri()->getBasePath();

    $extensions = ['jpg', 'jpeg', 'gif', 'png', 'css', 'js', 'ico'];
    $mimeTypes = [
        'jpg' => 'image/jpeg',
        'jpeg' => 'image/jpeg',
        'gif' => 'image/gif',
        'png' => 'image/png',
        'css' => 'text/css',
        'js' => 'application/javascript',
        'ico' => 'image/x-icon',
    ];

    $ext = pathinfo($file, PATHINFO_EXTENSION);
    if (false === in_array($ext, $extensions)) {
        return $next($request, $response);
    }

    $filePath = $file = __DIR__.'/../static'.$file;
    if (file_exists($filePath)) {
        $content = file_get_contents($filePath);
        $mimeType = $mimeTypes[$ext];
        $body = $response->getBody();
        $body->write($content);

        return $response->withBody($body)
            ->withHeader('Content-Type', $mimeType);
    }

    $next($request, $response);

    return $response;
};

$app->add($staticFileLoad);

require 'app.php';

$app->run();
