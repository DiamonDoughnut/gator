package config

import (
	"encoding/json"
	"os"
)

type Config struct{
	DbUrl string `json:"db_url"`
	CurrentUserName string `json:"current_user_name"`
}

func Read() (Config, error){
	configPath, err := os.UserHomeDir()
	if err != nil{
		return Config{}, err
	}
	configPath +=  "/.gatorconfig.json"
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
	configPath, err := os.UserHomeDir()
	if err != nil{
		return err
	}
	configPath +=  "/.gatorconfig.json"
	configFile, err := os.ReadFile(configPath)
	if err != nil{
		return err
	}
	var data Config
	err = json.Unmarshal(configFile, &data)
	if err != nil{
		return err
	}
	data.CurrentUserName = user
	configFile, err = json.Marshal(data)
	if err != nil{
		return err
	}
	err = os.WriteFile(configPath, configFile, 0644)
	if err != nil{
		return err
	}
	return nil
}