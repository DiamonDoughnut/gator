package config

import (
	"encoding/json"
	"os"
)

type Config struct{
	DbUrl string `json:"db_url"`
	CurrentUserName string `json:"current_user_name"`
}

func ConfigPath() (string, error){
	configPath, err := os.UserHomeDir()
	if err != nil{
		return "", err
	}
	configPath +=  "/.gatorconfig.json"
	return configPath, nil
}

func Read() (Config, error){
	configPath, err := ConfigPath()
	if err != nil{
		return Config{}, err
	}
	configFile, err := os.ReadFile(configPath)
	if err != nil{
		return Config{}, err
	}
	var data Config
	err = json.Unmarshal(configFile, &data)
	if err != nil{
		return Config{}, err
	}
	return data, nil
}

func SetUser(user string) error {
	data, err := Read()
	if err != nil{
		return err
	}
	
	data.CurrentUserName = user
	configFile, err := json.Marshal(data)
	if err != nil{
		return err
	}
	configPath, err := ConfigPath()
	if err != nil{
		return err
	}
	err = os.WriteFile(configPath, configFile, 0600)
	if err != nil{
		return err
	}
	return nil
}