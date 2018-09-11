<?php

require 'vendor/autoload.php';

$app = new \Slim\App();
require 'app.php';
$app->run();
